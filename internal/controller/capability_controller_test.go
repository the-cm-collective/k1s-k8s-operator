package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	k1sclient "github.com/k1s-project/k1s-operator/internal/k1s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildInferenceManifestSingleCell(t *testing.T) {
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "llama", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sInferenceEndpointSpec{
			Model:   operatorv1alpha1.K1sInferenceModelSpec{ModelID: "llama-3.1-8b", LocalPath: "/models/llama"},
			Members: []operatorv1alpha1.K1sInferenceMemberSpec{{SiteID: "edge-a", NodeID: "gpu-1", GPUCount: 1}},
		},
	}
	raw, err := buildInferenceManifest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "InferenceCell" {
		t.Fatalf("unexpected kind: %v", payload["kind"])
	}
	spec := payload["spec"].(map[string]any)
	model := spec["model"].(map[string]any)
	if model["modelId"] != "llama-3.1-8b" {
		t.Fatalf("unexpected model: %#v", model)
	}
	executor := spec["executor"].(map[string]any)
	if _, ok := executor["fallbackMode"]; ok {
		t.Fatalf("empty optional executor fields should be omitted: %#v", executor)
	}
}

func TestBuildInferenceManifestCellSet(t *testing.T) {
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sInferenceEndpointSpec{
			Model:   operatorv1alpha1.K1sInferenceModelSpec{ModelID: "model"},
			CellSet: &operatorv1alpha1.K1sInferenceCellSetSpec{Replicas: 2},
		},
	}
	raw, err := buildInferenceManifest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if kindFromManifest(raw) != "InferenceCellSet" {
		t.Fatalf("expected InferenceCellSet manifest: %s", raw)
	}
}

func TestAppNameFromManifestNamespaces(t *testing.T) {
	raw := []byte(`{"kind":"Deployment","metadata":{"namespace":"apps","name":"echo"}}`)
	if got := appNameFromManifest(raw); got != "apps--echo" {
		t.Fatalf("unexpected app name: %q", got)
	}
	namespace, name := appRefFromManifest([]byte(`{"kind":"Deployment","metadata":{"name":"echo"}}`))
	if namespace != "default" || name != "echo" {
		t.Fatalf("unexpected ref: %s/%s", namespace, name)
	}
}

func TestApplyObservedAppStatus(t *testing.T) {
	status := operatorv1alpha1.K1sAppStatus{AppName: "fallback", Phase: "accepted"}
	applyObservedAppStatus(&status, k1sclient.AppStatus{
		AppName:         "apps--echo",
		Ready:           true,
		DesiredReplicas: 2,
		ReadyReplicas:   2,
		LiveReplicas:    2,
		Revision:        "7",
		RevisionStatus:  "ready",
		Image:           "example/echo:latest",
		IngressHost:     "echo.example.test",
		IngressPath:     "/api",
	})
	if !status.Ready || status.Phase != "ready" || status.Endpoint != "echo.example.test/api" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.Replicas.Ready != 2 || status.Image == "" || status.Revision != "7" {
		t.Fatalf("missing observed fields: %#v", status)
	}
}

func TestInferenceEndpointReportsUnsupportedWhenK1sRejectsApply(t *testing.T) {
	ctx := context.Background()
	var sawInferenceManifest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apply" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		sawInferenceManifest = payload["kind"] == "InferenceCell"
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unsupported kind InferenceCell"}`))
	}))
	defer server.Close()

	scheme := runtime.NewScheme()
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &operatorv1alpha1.K1sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sClusterSpec{
			Controller: operatorv1alpha1.K1sEndpointSpec{URL: server.URL},
			AuthSecretRef: &operatorv1alpha1.K1sAuthSecretRef{
				Name:                    "k1s-creds",
				ControllerWriteTokenKey: "write",
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "k1s-creds", Namespace: "ml"},
		Data:       map[string][]byte{"write": []byte("token")},
	}
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "llama", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sInferenceEndpointSpec{
			ClusterRef: operatorv1alpha1.NamespacedNameRef{Name: "dev-a"},
			Model:      operatorv1alpha1.K1sInferenceModelSpec{ModelID: "llama-3.2-1b"},
		},
	}
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, secret, endpoint).
		WithStatusSubresource(&operatorv1alpha1.K1sInferenceEndpoint{}).
		Build()
	reconciler := &K1sInferenceEndpointReconciler{Client: kube, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ml", Name: "llama"}}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	got := &operatorv1alpha1.K1sInferenceEndpoint{}
	if err := kube.Get(ctx, req.NamespacedName, got); err != nil {
		t.Fatal(err)
	}
	if !sawInferenceManifest {
		t.Fatalf("expected reconciler to render an InferenceCell manifest")
	}
	if got.Status.Phase != "Unsupported" || got.Status.Ready {
		t.Fatalf("unexpected status: %#v", got.Status)
	}
	if !strings.Contains(got.Status.LastError, "unsupported kind InferenceCell") {
		t.Fatalf("missing apply error in status: %#v", got.Status)
	}
	applied := findCondition(got.Status.Conditions, operatorv1alpha1.ConditionApplied)
	if applied == nil || applied.Status != metav1.ConditionFalse || applied.Reason != operatorv1alpha1.ReasonUnsupported {
		t.Fatalf("unexpected applied condition: %#v", applied)
	}
}

func TestInferenceEndpointMapsObservedK1sStatus(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apply":
			_, _ = w.Write([]byte(`{"kind":"InferenceCell","name":"llama","namespace":"ml","phase":"PROGRESSING","status":"progressing"}`))
		case "/inference/cells/ml/llama":
			_, _ = w.Write([]byte(`{"kind":"InferenceCell","name":"llama","namespace":"ml","phase":"READY","status":"ready","ready":true,"api_endpoint":"10.0.0.10:18080","active_executor":"ray"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	scheme := runtime.NewScheme()
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &operatorv1alpha1.K1sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sClusterSpec{
			Controller: operatorv1alpha1.K1sEndpointSpec{URL: server.URL},
			AuthSecretRef: &operatorv1alpha1.K1sAuthSecretRef{
				Name:                    "k1s-creds",
				ControllerWriteTokenKey: "write",
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "k1s-creds", Namespace: "ml"},
		Data:       map[string][]byte{"write": []byte("token")},
	}
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "llama", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sInferenceEndpointSpec{
			ClusterRef: operatorv1alpha1.NamespacedNameRef{Name: "dev-a"},
			Model:      operatorv1alpha1.K1sInferenceModelSpec{ModelID: "llama-3.2-1b", LocalPath: "/models/llama"},
		},
	}
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, secret, endpoint).
		WithStatusSubresource(&operatorv1alpha1.K1sInferenceEndpoint{}).
		Build()
	reconciler := &K1sInferenceEndpointReconciler{Client: kube, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ml", Name: "llama"}}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	got := &operatorv1alpha1.K1sInferenceEndpoint{}
	if err := kube.Get(ctx, req.NamespacedName, got); err != nil {
		t.Fatal(err)
	}
	if !got.Status.Ready || got.Status.Phase != "READY" || got.Status.APIEndpoint != "10.0.0.10:18080" || got.Status.ActiveExecutor != "ray" {
		t.Fatalf("unexpected observed status: %#v", got.Status)
	}
	applied := findCondition(got.Status.Conditions, operatorv1alpha1.ConditionApplied)
	if applied == nil || applied.Status != metav1.ConditionTrue {
		t.Fatalf("unexpected applied condition: %#v", applied)
	}
}

func TestK1sAppDoesNotReapplyUnchangedManifestOnPoll(t *testing.T) {
	ctx := context.Background()
	var applyCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apply":
			applyCount++
			_, _ = w.Write([]byte(`{"app":"echo","status":"ready"}`))
		case "/status/echo":
			_, _ = w.Write([]byte(`{"app_name":"echo","revision_status":"ready","ready":true,"desired_replicas":1,"ready_replicas":1,"live_replicas":1}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	scheme := runtime.NewScheme()
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &operatorv1alpha1.K1sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a", Namespace: "apps"},
		Spec: operatorv1alpha1.K1sClusterSpec{
			Controller: operatorv1alpha1.K1sEndpointSpec{URL: server.URL},
			AuthSecretRef: &operatorv1alpha1.K1sAuthSecretRef{
				Name:                    "k1s-creds",
				ControllerWriteTokenKey: "write",
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "k1s-creds", Namespace: "apps"},
		Data:       map[string][]byte{"write": []byte("token")},
	}
	app := &operatorv1alpha1.K1sApp{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps", Finalizers: []string{capabilityFinalizer}},
		Spec: operatorv1alpha1.K1sAppSpec{
			ClusterRef: operatorv1alpha1.NamespacedNameRef{Name: "dev-a"},
			Manifest:   runtime.RawExtension{Raw: []byte(`{"apiVersion":"ae.dev/v1alpha1","kind":"Deployment","metadata":{"name":"echo"},"spec":{"image":"hashicorp/http-echo"}}`)},
		},
	}
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, secret, app).
		WithStatusSubresource(&operatorv1alpha1.K1sApp{}).
		Build()
	reconciler := &K1sAppReconciler{Client: kube, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "apps", Name: "echo"}}

	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if applyCount != 1 {
		t.Fatalf("expected one apply for unchanged manifest, got %d", applyCount)
	}
}

func TestK1sAppKeepsFinalizerWhenRemoteDeleteFails(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/delete/echo" {
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"delete failed"}`))
	}))
	defer server.Close()

	scheme := runtime.NewScheme()
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	deletedAt := metav1.Now()
	cluster := &operatorv1alpha1.K1sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a", Namespace: "apps"},
		Spec: operatorv1alpha1.K1sClusterSpec{
			Controller: operatorv1alpha1.K1sEndpointSpec{URL: server.URL},
			AuthSecretRef: &operatorv1alpha1.K1sAuthSecretRef{
				Name:                    "k1s-creds",
				ControllerWriteTokenKey: "write",
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "k1s-creds", Namespace: "apps"},
		Data:       map[string][]byte{"write": []byte("token")},
	}
	app := &operatorv1alpha1.K1sApp{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps", Finalizers: []string{capabilityFinalizer}, DeletionTimestamp: &deletedAt},
		Spec: operatorv1alpha1.K1sAppSpec{
			ClusterRef: operatorv1alpha1.NamespacedNameRef{Name: "dev-a"},
			Manifest:   runtime.RawExtension{Raw: []byte(`{"apiVersion":"ae.dev/v1alpha1","kind":"Deployment","metadata":{"name":"echo"},"spec":{"image":"hashicorp/http-echo"}}`)},
		},
	}
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, secret, app).
		WithStatusSubresource(&operatorv1alpha1.K1sApp{}).
		Build()
	reconciler := &K1sAppReconciler{Client: kube, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "apps", Name: "echo"}}

	if _, err := reconciler.Reconcile(ctx, req); err == nil {
		t.Fatal("expected delete failure")
	}
	got := &operatorv1alpha1.K1sApp{}
	if err := kube.Get(ctx, req.NamespacedName, got); err != nil {
		t.Fatal(err)
	}
	if !containsString(got.Finalizers, capabilityFinalizer) {
		t.Fatalf("expected finalizer to remain after failed remote delete: %#v", got.Finalizers)
	}
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
