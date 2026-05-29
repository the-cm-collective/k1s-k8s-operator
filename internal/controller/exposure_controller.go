package controller

import (
	"context"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	k1sclient "github.com/k1s-project/k1s-operator/internal/k1s"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type K1sExposureReconciler struct {
	client.Client
	Reader       client.Reader
	Scheme       *runtime.Scheme
	TrafficProbe TrafficProbeFunc
}

// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sexposures,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sclusters;k1sappmirrors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sappmirrors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
func (r *K1sExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	exposure := &operatorv1alpha1.K1sExposure{}
	if err := r.Get(ctx, req.NamespacedName, exposure); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	patchExposureStatus := func() error {
		return patchStatus(ctx, r.Client, exposure, func(obj client.Object) {
			obj.(*operatorv1alpha1.K1sExposure).Status = exposure.Status
		})
	}
	cluster := &operatorv1alpha1.K1sCluster{}
	clusterNamespace := exposure.Spec.ClusterRef.NamespaceOr(exposure.Namespace)
	if err := r.Get(ctx, types.NamespacedName{Name: exposure.Spec.ClusterRef.Name, Namespace: clusterNamespace}, cluster); err != nil {
		exposure.Status.ObservedGeneration = exposure.Generation
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionClusterReady, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), exposure.Generation)
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, "referenced cluster is not ready", exposure.Generation)
		return ctrl.Result{}, patchExposureStatus()
	}
	creds, err := loadCredentials(ctx, r.Client, cluster.Namespace, cluster.Spec.AuthSecretRef)
	if err != nil {
		exposure.Status.ObservedGeneration = exposure.Generation
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionClusterReady, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), exposure.Generation)
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, "cluster credentials are not available", exposure.Generation)
		return ctrl.Result{}, patchExposureStatus()
	}
	api, err := k1sclient.NewClient(cluster.Spec.Controller.URL, k1sclient.WithBearerToken(creds.readToken), k1sclient.WithCABundle(creds.caBundle))
	if err != nil {
		exposure.Status.ObservedGeneration = exposure.Generation
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionClusterReady, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, err.Error(), exposure.Generation)
		return ctrl.Result{}, patchExposureStatus()
	}
	appStatus, appErr := api.AppStatus(ctx, exposure.Spec.AppRef.NamespaceOrDefault(), exposure.Spec.AppRef.Name)
	appFound := appErr == nil
	appReady := appStatus.Ready
	setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionAppFound, conditionStatus(appFound), reasonForBool(appFound), appMessage(appFound, appErr), exposure.Generation)
	setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionAppReady, conditionStatus(appReady), reasonForBool(appReady), appReadyMessage(appReady), exposure.Generation)
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	proxyEndpoints, proxyErr := discoverProxyEndpoints(ctx, reader, cluster)
	proxyReady := proxyErr == nil && len(proxyEndpoints) > 0
	trafficEndpoints := proxyEndpoints
	if !appFound {
		trafficEndpoints = nil
	}
	setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionProxyEndpointsReady, conditionStatus(proxyReady), reasonForBool(proxyReady), proxyMessage(len(proxyEndpoints)), exposure.Generation)
	if err := r.reconcileService(ctx, cluster, exposure); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileEndpointSlice(ctx, cluster, exposure, trafficEndpoints); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileIngress(ctx, cluster, exposure); err != nil {
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionIngressReady, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, err.Error(), exposure.Generation)
	} else {
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionIngressReady, metav1.ConditionTrue, operatorv1alpha1.ReasonApplied, "ingress reconciled", exposure.Generation)
	}
	if err := r.reconcileMirror(ctx, cluster, exposure, appStatus, appFound); err != nil {
		logger.Error(err, "unable to reconcile app mirror")
		return ctrl.Result{}, err
	}
	now := metav1.Now()
	exposure.Status.ObservedGeneration = exposure.Generation
	exposure.Status.AppReady = appReady
	exposure.Status.ServiceName = exposureServiceName(exposure)
	exposure.Status.EndpointSliceName = exposureEndpointSliceName(exposure)
	exposure.Status.IngressName = ""
	if exposure.Spec.Ingress.EnabledOrDefault() {
		exposure.Status.IngressName = exposureIngressName(exposure)
	}
	exposure.Status.ProxyEndpointCount = int32(len(trafficEndpoints))
	exposure.Status.LastAppSyncTime = &now
	setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionResourcesApplied, metav1.ConditionTrue, operatorv1alpha1.ReasonApplied, "generated resources reconciled", exposure.Generation)
	setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionClusterReady, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "referenced cluster loaded", exposure.Generation)
	trafficReady := !exposure.Spec.TrafficProbe.EnabledOrDefault()
	if exposure.Spec.TrafficProbe.EnabledOrDefault() {
		probe := r.TrafficProbe
		if probe == nil {
			probe = defaultTrafficProbe
		}
		if err := probe(ctx, exposure); err != nil {
			setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionTrafficReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, err.Error(), exposure.Generation)
			trafficReady = false
		} else {
			setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionTrafficReady, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "traffic probe succeeded", exposure.Generation)
			trafficReady = true
		}
	} else {
		setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionTrafficReady, metav1.ConditionUnknown, operatorv1alpha1.ReasonPending, "traffic probe disabled", exposure.Generation)
	}
	ready := appFound && appReady && proxyReady && trafficReady
	setCondition(&exposure.Status.Conditions, operatorv1alpha1.ConditionReady, conditionStatus(ready), reasonForBool(ready), readyMessage(ready), exposure.Generation)
	return ctrl.Result{RequeueAfter: pollInterval(cluster.Spec.PollIntervalSeconds)}, patchExposureStatus()
}

func (r *K1sExposureReconciler) reconcileService(ctx context.Context, cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure) error {
	wanted := BuildExposureService(cluster, exposure)
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: wanted.Name, Namespace: wanted.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		if err := controllerutil.SetControllerReference(exposure, current, r.Scheme); err != nil {
			return err
		}
		current.Labels = wanted.Labels
		current.Spec.Type = wanted.Spec.Type
		current.Spec.Selector = nil
		current.Spec.Ports = wanted.Spec.Ports
		return nil
	})
	return err
}

func (r *K1sExposureReconciler) reconcileEndpointSlice(ctx context.Context, cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure, endpoints []ProxyEndpoint) error {
	wanted := BuildExposureEndpointSlice(cluster, exposure, endpoints)
	current := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: wanted.Name, Namespace: wanted.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		if err := controllerutil.SetControllerReference(exposure, current, r.Scheme); err != nil {
			return err
		}
		current.Labels = wanted.Labels
		current.AddressType = wanted.AddressType
		current.Ports = wanted.Ports
		current.Endpoints = wanted.Endpoints
		return nil
	})
	return err
}

func (r *K1sExposureReconciler) reconcileIngress(ctx context.Context, cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure) error {
	wanted, err := BuildExposureIngress(cluster, exposure)
	if err != nil {
		return err
	}
	if wanted == nil {
		current := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: exposureIngressName(exposure), Namespace: exposure.Namespace}}
		if err := r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}
	current := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: wanted.Name, Namespace: wanted.Namespace}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		if err := controllerutil.SetControllerReference(exposure, current, r.Scheme); err != nil {
			return err
		}
		current.Labels = wanted.Labels
		current.Annotations = wanted.Annotations
		current.Spec = wanted.Spec
		return nil
	})
	return err
}

func (r *K1sExposureReconciler) reconcileMirror(ctx context.Context, cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure, appStatus k1sclient.AppStatus, appFound bool) error {
	mirror := &operatorv1alpha1.K1sAppMirror{ObjectMeta: metav1.ObjectMeta{Name: exposure.Name, Namespace: exposure.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, mirror, func() error {
		if err := controllerutil.SetControllerReference(exposure, mirror, r.Scheme); err != nil {
			return err
		}
		mirror.Spec = operatorv1alpha1.K1sAppMirrorSpec{
			ClusterRef:  exposure.Spec.ClusterRef,
			AppRef:      exposure.Spec.AppRef,
			ExposureRef: &operatorv1alpha1.NamespacedNameRef{Name: exposure.Name},
		}
		_ = cluster
		return nil
	})
	if err != nil {
		return err
	}
	return patchStatus(ctx, r.Client, mirror, func(obj client.Object) {
		current := obj.(*operatorv1alpha1.K1sAppMirror)
		current.Status = mirrorStatusFromApp(current, appStatus, appFound)
	})
}

func (r *K1sExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.K1sExposure{}).
		Owns(&corev1.Service{}).
		Owns(&discoveryv1.EndpointSlice{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&operatorv1alpha1.K1sAppMirror{}).
		Complete(r)
}

func appMessage(found bool, err error) string {
	if found {
		return "k1s app status was read"
	}
	if err == nil {
		return "k1s app was not found"
	}
	return err.Error()
}

func appReadyMessage(ready bool) string {
	if ready {
		return "k1s app is ready"
	}
	return "k1s app is not ready"
}

func mirrorStatusFromApp(mirror *operatorv1alpha1.K1sAppMirror, appStatus k1sclient.AppStatus, appFound bool) operatorv1alpha1.K1sAppMirrorStatus {
	now := metav1.Now()
	status := operatorv1alpha1.K1sAppMirrorStatus{
		ObservedGeneration: mirror.Generation,
		Ready:              appFound && appStatus.Ready,
		Replicas:           operatorv1alpha1.K1sReplicaSummary{Desired: appStatus.DesiredReplicas, Ready: appStatus.ReadyReplicas, Live: appStatus.LiveReplicas},
		Image:              appStatus.Image,
		Revision:           appStatus.Revision,
		ObservedHost:       appStatus.IngressHost,
		LastSyncTime:       &now,
	}
	for _, placement := range appStatus.Placements {
		status.Placements = append(status.Placements, operatorv1alpha1.K1sPlacementStatus{Node: placement.Node, Ready: placement.Ready})
	}
	setCondition(&status.Conditions, operatorv1alpha1.ConditionReady, conditionStatus(status.Ready), reasonForBool(status.Ready), readyMessage(status.Ready), mirror.Generation)
	return status
}
