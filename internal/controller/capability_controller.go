package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	k1sclient "github.com/k1s-project/k1s-operator/internal/k1s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const capabilityFinalizer = "operator.k1s.io/capability-cleanup"

type K1sAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sapps,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
func (r *K1sAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	app := &operatorv1alpha1.K1sApp{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	api, cluster, err := r.writeClient(ctx, app.Namespace, app.Spec.ClusterRef)
	if err != nil {
		app.Status.ObservedGeneration = app.Generation
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionCredentialsValid, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), app.Generation)
		return ctrl.Result{}, r.Status().Update(ctx, app)
	}
	if !app.ObjectMeta.DeletionTimestamp.IsZero() {
		if containsString(app.Finalizers, capabilityFinalizer) {
			if app.Spec.DeletePolicy != operatorv1alpha1.K1sDeletePolicyOrphan {
				_, _ = api.DeleteApp(ctx, appDeleteName(app), true)
			}
			app.Finalizers = removeString(app.Finalizers, capabilityFinalizer)
			return ctrl.Result{}, r.Update(ctx, app)
		}
		return ctrl.Result{}, nil
	}
	if !containsString(app.Finalizers, capabilityFinalizer) {
		app.Finalizers = append(app.Finalizers, capabilityFinalizer)
		if err := r.Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	raw := app.Spec.Manifest.Raw
	if len(raw) == 0 {
		app.Status.ObservedGeneration = app.Generation
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionAccepted, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, "manifest is required", app.Generation)
		return ctrl.Result{}, r.Status().Update(ctx, app)
	}
	if kind := kindFromManifest(raw); kind != "Deployment" {
		app.Status.ObservedGeneration = app.Generation
		app.Status.AppName = appNameFromManifest(raw)
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionAccepted, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, fmt.Sprintf("K1sApp supports kind Deployment, got %q", kind), app.Generation)
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionReady, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, "resource is not ready", app.Generation)
		return ctrl.Result{}, r.Status().Update(ctx, app)
	}
	result, err := api.Apply(ctx, raw)
	now := metav1.Now()
	app.Status.ObservedGeneration = app.Generation
	app.Status.LastSyncTime = &now
	app.Status.AppName = firstNonEmpty(strMap(result, "app"), appNameFromManifest(raw), app.Name)
	app.Status.Phase = firstNonEmpty(strMap(result, "status"), "Applied")
	app.Status.Ready = strings.EqualFold(app.Status.Phase, "ready") || strings.EqualFold(app.Status.Phase, "live")
	setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionAccepted, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "K1sApp accepted by operator", app.Generation)
	if err != nil {
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionApplied, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, err.Error(), app.Generation)
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionAppReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, "manifest was not applied", app.Generation)
		app.Status.Ready = false
	} else {
		setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionApplied, metav1.ConditionTrue, operatorv1alpha1.ReasonApplied, "manifest applied to k1s", app.Generation)
		namespace, name := appRefFromManifest(raw)
		if observed, statusErr := api.AppStatus(ctx, namespace, name); statusErr == nil {
			applyObservedAppStatus(&app.Status, observed)
			setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionAppReady, conditionStatus(app.Status.Ready), reasonForBool(app.Status.Ready), appReadyMessage(app.Status.Ready), app.Generation)
		} else {
			app.Status.Ready = false
			setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionAppReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, "manifest applied but app status could not be read: "+statusErr.Error(), app.Generation)
		}
	}
	setCondition(&app.Status.Conditions, operatorv1alpha1.ConditionReady, conditionStatus(app.Status.Ready), reasonForBool(app.Status.Ready), readyMessage(app.Status.Ready), app.Generation)
	_ = cluster
	return ctrl.Result{RequeueAfter: pollInterval(cluster.Spec.PollIntervalSeconds)}, r.Status().Update(ctx, app)
}

func (r *K1sAppReconciler) writeClient(ctx context.Context, namespace string, ref operatorv1alpha1.NamespacedNameRef) (*k1sclient.Client, *operatorv1alpha1.K1sCluster, error) {
	cluster := &operatorv1alpha1.K1sCluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.NamespaceOr(namespace)}, cluster); err != nil {
		return nil, nil, err
	}
	creds, err := loadCredentials(ctx, r.Client, cluster.Namespace, cluster.Spec.AuthSecretRef)
	if err != nil {
		return nil, nil, err
	}
	if creds.writeToken == "" {
		return nil, nil, fmt.Errorf("write token is required for k1s capability CRUD")
	}
	api, err := k1sclient.NewClient(cluster.Spec.Controller.URL, k1sclient.WithBearerToken(creds.writeToken), k1sclient.WithCABundle(creds.caBundle))
	return api, cluster, err
}

func (r *K1sAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&operatorv1alpha1.K1sApp{}).Complete(r)
}

type K1sInferenceEndpointReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sinferenceendpoints,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sinferenceendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sinferenceendpoints/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
func (r *K1sInferenceEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, endpoint); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	appReconciler := K1sAppReconciler{Client: r.Client, Scheme: r.Scheme}
	api, cluster, err := appReconciler.writeClient(ctx, endpoint.Namespace, endpoint.Spec.ClusterRef)
	if err != nil {
		endpoint.Status.ObservedGeneration = endpoint.Generation
		setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionCredentialsValid, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), endpoint.Generation)
		return ctrl.Result{}, r.Status().Update(ctx, endpoint)
	}
	if !endpoint.ObjectMeta.DeletionTimestamp.IsZero() {
		if containsString(endpoint.Finalizers, capabilityFinalizer) {
			if endpoint.Spec.DeletePolicy != operatorv1alpha1.K1sDeletePolicyOrphan {
				if endpoint.Spec.CellSet != nil {
					_, _ = api.DeleteInferenceCellSet(ctx, endpoint.Namespace, endpoint.Name)
				} else {
					_, _ = api.DeleteInferenceCell(ctx, endpoint.Namespace, endpoint.Name)
				}
			}
			endpoint.Finalizers = removeString(endpoint.Finalizers, capabilityFinalizer)
			return ctrl.Result{}, r.Update(ctx, endpoint)
		}
		return ctrl.Result{}, nil
	}
	if !containsString(endpoint.Finalizers, capabilityFinalizer) {
		endpoint.Finalizers = append(endpoint.Finalizers, capabilityFinalizer)
		if err := r.Update(ctx, endpoint); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	manifest, err := buildInferenceManifest(endpoint)
	if err != nil {
		endpoint.Status.ObservedGeneration = endpoint.Generation
		setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionAccepted, metav1.ConditionFalse, operatorv1alpha1.ReasonInvalid, err.Error(), endpoint.Generation)
		return ctrl.Result{}, r.Status().Update(ctx, endpoint)
	}
	result, applyErr := api.Apply(ctx, manifest)
	now := metav1.Now()
	endpoint.Status.ObservedGeneration = endpoint.Generation
	endpoint.Status.LastSyncTime = &now
	endpoint.Status.CellName = endpoint.Name
	endpoint.Status.CellSetName = ""
	if endpoint.Spec.CellSet != nil {
		endpoint.Status.CellName = ""
		endpoint.Status.CellSetName = endpoint.Name
	}
	endpoint.Status.Phase = firstNonEmpty(strMap(result, "phase"), strMap(result, "status"), "Applied")
	endpoint.Status.Ready = strings.EqualFold(endpoint.Status.Phase, "ready")
	endpoint.Status.APIEndpoint = strMap(result, "api_endpoint")
	endpoint.Status.ActiveExecutor = strMap(result, "active_executor")
	setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionAccepted, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "K1sInferenceEndpoint accepted by operator", endpoint.Generation)
	if applyErr != nil {
		endpoint.Status.Ready = false
		endpoint.Status.Phase = "Unsupported"
		endpoint.Status.LastError = applyErr.Error()
		setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionApplied, metav1.ConditionFalse, operatorv1alpha1.ReasonUnsupported, "k1s inference CRUD API is not available or rejected the request: "+applyErr.Error(), endpoint.Generation)
	} else {
		endpoint.Status.LastError = ""
		setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionApplied, metav1.ConditionTrue, operatorv1alpha1.ReasonApplied, "inference manifest applied to k1s", endpoint.Generation)
		if observed, statusErr := inferenceStatusForEndpoint(ctx, api, endpoint); statusErr == nil {
			applyObservedInferenceStatus(&endpoint.Status, observed)
		} else {
			endpoint.Status.Ready = false
			endpoint.Status.LastError = statusErr.Error()
			setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionReady, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, "manifest applied but inference status could not be read: "+statusErr.Error(), endpoint.Generation)
		}
	}
	setCondition(&endpoint.Status.Conditions, operatorv1alpha1.ConditionReady, conditionStatus(endpoint.Status.Ready), reasonForBool(endpoint.Status.Ready), readyMessage(endpoint.Status.Ready), endpoint.Generation)
	return ctrl.Result{RequeueAfter: pollInterval(cluster.Spec.PollIntervalSeconds)}, r.Status().Update(ctx, endpoint)
}

func (r *K1sInferenceEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&operatorv1alpha1.K1sInferenceEndpoint{}).Complete(r)
}

type K1sResourceSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sresourcesets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sresourcesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sresourcesets/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.k1s.io,resources=k1sclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
func (r *K1sResourceSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	set := &operatorv1alpha1.K1sResourceSet{}
	if err := r.Get(ctx, req.NamespacedName, set); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	appReconciler := K1sAppReconciler{Client: r.Client, Scheme: r.Scheme}
	api, cluster, err := appReconciler.writeClient(ctx, set.Namespace, set.Spec.ClusterRef)
	if err != nil {
		set.Status.ObservedGeneration = set.Generation
		setCondition(&set.Status.Conditions, operatorv1alpha1.ConditionCredentialsValid, metav1.ConditionFalse, operatorv1alpha1.ReasonMissing, err.Error(), set.Generation)
		return ctrl.Result{}, r.Status().Update(ctx, set)
	}
	allowed := set.Spec.AllowedKinds
	if len(allowed) == 0 {
		allowed = []string{"Deployment", "InferenceCell", "InferenceCellSet"}
	}
	var applied int32
	var firstErr error
	for _, manifest := range set.Spec.Manifests {
		kind := kindFromManifest(manifest.Raw)
		if !slices.Contains(allowed, kind) {
			firstErr = fmt.Errorf("kind %q is not allowed by this K1sResourceSet", kind)
			break
		}
		if _, err := api.Apply(ctx, manifest.Raw); err != nil {
			firstErr = err
			break
		}
		applied++
	}
	now := metav1.Now()
	set.Status.ObservedGeneration = set.Generation
	set.Status.LastSyncTime = &now
	set.Status.Applied = applied
	set.Status.Ready = firstErr == nil && applied == int32(len(set.Spec.Manifests))
	if firstErr != nil {
		setCondition(&set.Status.Conditions, operatorv1alpha1.ConditionPolicyAllowed, metav1.ConditionFalse, operatorv1alpha1.ReasonDenied, firstErr.Error(), set.Generation)
		setCondition(&set.Status.Conditions, operatorv1alpha1.ConditionApplied, metav1.ConditionFalse, operatorv1alpha1.ReasonUnavailable, firstErr.Error(), set.Generation)
	} else {
		setCondition(&set.Status.Conditions, operatorv1alpha1.ConditionPolicyAllowed, metav1.ConditionTrue, operatorv1alpha1.ReasonReady, "all manifest kinds are allowed", set.Generation)
		setCondition(&set.Status.Conditions, operatorv1alpha1.ConditionApplied, metav1.ConditionTrue, operatorv1alpha1.ReasonApplied, "resource set applied to k1s", set.Generation)
	}
	setCondition(&set.Status.Conditions, operatorv1alpha1.ConditionReady, conditionStatus(set.Status.Ready), reasonForBool(set.Status.Ready), readyMessage(set.Status.Ready), set.Generation)
	return ctrl.Result{RequeueAfter: pollInterval(cluster.Spec.PollIntervalSeconds)}, r.Status().Update(ctx, set)
}

func (r *K1sResourceSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&operatorv1alpha1.K1sResourceSet{}).Complete(r)
}

func buildInferenceManifest(endpoint *operatorv1alpha1.K1sInferenceEndpoint) ([]byte, error) {
	if endpoint.Spec.Model.ModelID == "" {
		return nil, fmt.Errorf("spec.model.modelId is required")
	}
	executor := map[string]any{
		"type": firstNonEmpty(endpoint.Spec.Executor.Type, "ray"),
	}
	addString(executor, "fallbackMode", endpoint.Spec.Executor.FallbackMode)
	addString(executor, "rayImage", endpoint.Spec.Executor.RayImage)
	addString(executor, "mpImage", endpoint.Spec.Executor.MPImage)
	addString(executor, "launcherImage", endpoint.Spec.Executor.LauncherImage)
	addString(executor, "dtype", endpoint.Spec.Executor.DType)
	addString(executor, "runtimeClassName", endpoint.Spec.Executor.RuntimeClassName)
	spec := map[string]any{
		"model": map[string]any{
			"modelId":   endpoint.Spec.Model.ModelID,
			"localPath": endpoint.Spec.Model.LocalPath,
		},
		"parallelism": map[string]any{
			"tp": defaultI32(endpoint.Spec.Parallelism.TP, 1),
			"pp": defaultI32(endpoint.Spec.Parallelism.PP, 1),
		},
		"executor": executor,
		"fabric": map[string]any{
			"mode":       firstNonEmpty(endpoint.Spec.Fabric.Mode, "lan_direct"),
			"policyMode": firstNonEmpty(endpoint.Spec.Fabric.PolicyMode, "strict_membership"),
			"ttlSeconds": defaultI32(endpoint.Spec.Fabric.TTLSeconds, 300),
		},
		"members":     endpoint.Spec.Members,
		"linkMetrics": []any{},
	}
	kind := "InferenceCell"
	if endpoint.Spec.CellSet != nil {
		kind = "InferenceCellSet"
		spec = map[string]any{
			"replicas":   defaultI32(endpoint.Spec.CellSet.Replicas, 1),
			"nameFormat": firstNonEmpty(endpoint.Spec.CellSet.NameFormat, "{set}-{i:03d}"),
			"template":   spec,
		}
	}
	payload := map[string]any{
		"apiVersion": "ae.dev/v1alpha1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":      endpoint.Name,
			"namespace": endpoint.Namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by":       "k1s-operator",
				"operator.k1s.io/inference-endpoint": endpoint.Name,
			},
		},
		"spec": spec,
	}
	return json.Marshal(payload)
}

func addString(payload map[string]any, key, value string) {
	if value != "" {
		payload[key] = value
	}
}

func kindFromManifest(raw []byte) string {
	var payload struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(raw, &payload)
	return payload.Kind
}

func appNameFromManifest(raw []byte) string {
	namespace, name := appRefFromManifest(raw)
	if namespace != "" && namespace != "default" {
		return namespace + "--" + name
	}
	return name
}

func appRefFromManifest(raw []byte) (string, string) {
	var payload struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(raw, &payload)
	namespace := payload.Metadata.Namespace
	if namespace == "" {
		namespace = "default"
	}
	return namespace, payload.Metadata.Name
}

func appDeleteName(app *operatorv1alpha1.K1sApp) string {
	return firstNonEmpty(app.Status.AppName, appNameFromManifest(app.Spec.Manifest.Raw), app.Name)
}

func applyObservedAppStatus(status *operatorv1alpha1.K1sAppStatus, observed k1sclient.AppStatus) {
	status.AppName = firstNonEmpty(observed.AppName, status.AppName)
	status.Ready = observed.Ready
	status.Phase = firstNonEmpty(observed.RevisionStatus, status.Phase)
	status.Endpoint = observedEndpoint(observed)
	status.Replicas = operatorv1alpha1.K1sReplicaSummary{
		Desired: observed.DesiredReplicas,
		Ready:   observed.ReadyReplicas,
		Live:    observed.LiveReplicas,
	}
	status.Image = observed.Image
	status.Revision = observed.Revision
}

func inferenceStatusForEndpoint(ctx context.Context, api *k1sclient.Client, endpoint *operatorv1alpha1.K1sInferenceEndpoint) (k1sclient.InferenceStatus, error) {
	if endpoint.Spec.CellSet != nil {
		return api.InferenceCellSetStatus(ctx, endpoint.Namespace, endpoint.Name)
	}
	return api.InferenceCellStatus(ctx, endpoint.Namespace, endpoint.Name)
}

func applyObservedInferenceStatus(status *operatorv1alpha1.K1sInferenceEndpointStatus, observed k1sclient.InferenceStatus) {
	status.Ready = observed.Ready
	status.Phase = firstNonEmpty(observed.Phase, observed.Status, status.Phase)
	status.APIEndpoint = firstNonEmpty(observed.APIEndpoint, status.APIEndpoint)
	status.ActiveExecutor = firstNonEmpty(observed.ActiveExecutor, status.ActiveExecutor)
	status.LastError = observed.LastError
	if observed.Kind == "InferenceCellSet" {
		status.CellName = ""
		status.CellSetName = firstNonEmpty(observed.Name, status.CellSetName)
		return
	}
	status.CellName = firstNonEmpty(observed.Name, status.CellName)
}

func observedEndpoint(observed k1sclient.AppStatus) string {
	if observed.IngressHost == "" {
		return ""
	}
	if observed.IngressPath == "" || observed.IngressPath == "/" {
		return observed.IngressHost
	}
	return observed.IngressHost + observed.IngressPath
}

func strMap(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

func defaultI32(value, fallback int32) int32 {
	if value == 0 {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func containsString(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}

func removeString(values []string, unwanted string) []string {
	var out []string
	for _, value := range values {
		if value != unwanted {
			out = append(out, value)
		}
	}
	return out
}
