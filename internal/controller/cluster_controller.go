package controller

import (
	"context"
	"time"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	k1sclient "github.com/k1s-project/k1s-operator/internal/k1s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type K1sClusterReconciler struct {
	client.Client
	Reader client.Reader
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sclusters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;services,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
func (r *K1sClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cluster := &operatorv1alpha1.K1sCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	status := operatorv1alpha1.K1sClusterStatus{ObservedGeneration: cluster.Generation}
	creds, err := loadCredentials(ctx, r.Client, cluster.Namespace, cluster.Spec.AuthSecretRef)
	if err != nil {
		setCondition(&status.Conditions, operatorv1alpha1.ConditionCredentialsValid, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), cluster.Generation)
	} else {
		setCondition(&status.Conditions, operatorv1alpha1.ConditionCredentialsValid, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "credentials loaded", cluster.Generation)
	}
	if cluster.Spec.BootstrapConfigMapRef != nil {
		cm := &corev1.ConfigMap{}
		ref := cluster.Spec.BootstrapConfigMapRef
		reader := r.Reader
		if reader == nil {
			reader = r.Client
		}
		if err := reader.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.NamespaceOr(cluster.Namespace)}, cm); err != nil {
			setCondition(&status.Conditions, operatorv1alpha1.ConditionBootstrapLoaded, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), cluster.Generation)
		} else {
			status.StackDomain = cm.Data["stack_domain"]
			status.WildcardAppsDomain = cm.Data["wildcard_apps_domain"]
			setCondition(&status.Conditions, operatorv1alpha1.ConditionBootstrapLoaded, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "bootstrap loaded", cluster.Generation)
		}
	} else {
		setCondition(&status.Conditions, operatorv1alpha1.ConditionBootstrapLoaded, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, "bootstrap configmap not configured", cluster.Generation)
	}
	if err == nil {
		api, clientErr := k1sclient.NewClient(cluster.Spec.Controller.URL, k1sclient.WithBearerToken(creds.readToken), k1sclient.WithCABundle(creds.caBundle))
		if clientErr != nil {
			setCondition(&status.Conditions, operatorv1alpha1.ConditionControllerAvailable, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, clientErr.Error(), cluster.Generation)
		} else if healthErr := api.Health(ctx); healthErr != nil {
			setCondition(&status.Conditions, operatorv1alpha1.ConditionControllerAvailable, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, healthErr.Error(), cluster.Generation)
		} else {
			status.ControllerAvailable = true
			setCondition(&status.Conditions, operatorv1alpha1.ConditionControllerAvailable, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "controller API is reachable", cluster.Generation)
			if nodes, nodeErr := api.Nodes(ctx); nodeErr == nil {
				status.NodeSummary = operatorv1alpha1.K1sNodeSummary{Ready: nodes.Ready, Stale: nodes.Stale, Total: nodes.Total}
			}
		}
	}
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	endpoints, proxyErr := discoverProxyEndpoints(ctx, reader, cluster)
	if proxyErr != nil {
		setCondition(&status.Conditions, operatorv1alpha1.ConditionProxyReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, proxyErr.Error(), cluster.Generation)
	} else {
		status.ProxyReady = len(endpoints) > 0
		setCondition(&status.Conditions, operatorv1alpha1.ConditionProxyReady, conditionStatus(status.ProxyReady), reasonForBool(status.ProxyReady), proxyMessage(len(endpoints)), cluster.Generation)
	}
	if cluster.Spec.Apishim != nil {
		status.ApishimAvailable = false
		setCondition(&status.Conditions, operatorv1alpha1.ConditionApishimAvailable, metav1.ConditionFalse, operatorv1alpha1.ReasonPending, "apishim URL configured; active apishim health check is deferred", cluster.Generation)
	}
	ready := status.ControllerAvailable && status.ProxyReady && err == nil
	setCondition(&status.Conditions, operatorv1alpha1.ConditionReady, conditionStatus(ready), reasonForBool(ready), clusterReadyMessage(ready), cluster.Generation)
	if updateErr := patchStatus(ctx, r.Client, cluster, func(obj client.Object) {
		obj.(*operatorv1alpha1.K1sCluster).Status = status
	}); updateErr != nil {
		logger.Error(updateErr, "unable to update K1sCluster status")
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{RequeueAfter: pollInterval(cluster.Spec.PollIntervalSeconds)}, nil
}

func (r *K1sClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.K1sCluster{}).
		Complete(r)
}

func pollInterval(raw *int32) time.Duration {
	if raw == nil || *raw <= 0 {
		return 30 * time.Second
	}
	return time.Duration(*raw) * time.Second
}

func reasonForBool(ok bool) string {
	if ok {
		return operatorv1alpha1.ReasonReady
	}
	return operatorv1alpha1.ReasonUnavailable
}

func proxyMessage(count int) string {
	if count == 0 {
		return "proxy has no ready endpoints"
	}
	return "proxy has ready endpoints"
}

func readyMessage(ok bool) string {
	if ok {
		return "resource is ready"
	}
	return "resource is not ready"
}

func clusterReadyMessage(ok bool) string {
	if ok {
		return "cluster is ready"
	}
	return "cluster is not ready"
}
