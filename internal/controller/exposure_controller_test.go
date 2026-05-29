package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileIngressDeletesExistingIngressWhenDisabled(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := networkingv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	exposure := &operatorv1alpha1.K1sExposure{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
		Spec: operatorv1alpha1.K1sExposureSpec{
			Ingress: operatorv1alpha1.K1sExposureIngressSpec{Enabled: ptr(false)},
		},
	}
	ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "echo-k1s", Namespace: "apps"}}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingress).Build()
	reconciler := &K1sExposureReconciler{Client: kube, Scheme: scheme}

	if err := reconciler.reconcileIngress(ctx, &operatorv1alpha1.K1sCluster{}, exposure); err != nil {
		t.Fatal(err)
	}
	got := &networkingv1.Ingress{}
	err := kube.Get(ctx, types.NamespacedName{Name: "echo-k1s", Namespace: "apps"}, got)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected generated ingress to be deleted, got err=%v ingress=%#v", err, got)
	}
}

func TestExpectedStatusRange(t *testing.T) {
	tests := map[string][2]int{
		"":        {200, 399},
		"204":     {204, 204},
		"200-299": {200, 299},
	}
	for raw, want := range tests {
		minStatus, maxStatus, err := expectedStatusRange(raw)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", raw, err)
		}
		if minStatus != want[0] || maxStatus != want[1] {
			t.Fatalf("unexpected range for %q: %d-%d", raw, minStatus, maxStatus)
		}
	}
	if _, _, err := expectedStatusRange("500-200"); err == nil {
		t.Fatal("expected inverted status range to fail")
	}
}

func TestExposureTrafficProbeControlsReadinessWhenEnabled(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/echo" {
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"app_name":"echo","revision_status":"ready","ready":true,"desired_replicas":1,"ready_replicas":1,"live_replicas":1}`))
	}))
	defer server.Close()

	run := func(t *testing.T, probe TrafficProbeFunc, wantReady metav1.ConditionStatus) {
		t.Helper()
		scheme := runtime.NewScheme()
		if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		if err := corev1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		if err := discoveryv1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		if err := networkingv1.AddToScheme(scheme); err != nil {
			t.Fatal(err)
		}
		cluster := &operatorv1alpha1.K1sCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-a", Namespace: "apps"},
			Spec: operatorv1alpha1.K1sClusterSpec{
				Controller: operatorv1alpha1.K1sEndpointSpec{URL: server.URL},
				Proxy: operatorv1alpha1.K1sProxySpec{
					Mode: operatorv1alpha1.K1sProxyModeExternal,
					External: &operatorv1alpha1.K1sProxyExternalSpec{
						Addresses: []string{"127.0.0.1"},
						Port:      10080,
					},
				},
			},
		}
		exposure := &operatorv1alpha1.K1sExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
			Spec: operatorv1alpha1.K1sExposureSpec{
				ClusterRef: operatorv1alpha1.NamespacedNameRef{Name: "dev-a"},
				AppRef:     operatorv1alpha1.K1sAppRef{Name: "echo"},
				Host:       "echo.example.test",
				TrafficProbe: operatorv1alpha1.K1sExposureTrafficProbeSpec{
					Enabled: ptr(true),
				},
			},
		}
		kube := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, exposure).
			WithStatusSubresource(&operatorv1alpha1.K1sExposure{}, &operatorv1alpha1.K1sAppMirror{}).
			Build()
		reconciler := &K1sExposureReconciler{Client: kube, Scheme: scheme, TrafficProbe: probe}
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "echo", Namespace: "apps"}}

		if _, err := reconciler.Reconcile(ctx, req); err != nil {
			t.Fatal(err)
		}
		got := &operatorv1alpha1.K1sExposure{}
		if err := kube.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatal(err)
		}
		ready := findCondition(got.Status.Conditions, operatorv1alpha1.ConditionReady)
		if ready == nil || ready.Status != wantReady {
			t.Fatalf("unexpected ready condition: %#v", ready)
		}
		traffic := findCondition(got.Status.Conditions, operatorv1alpha1.ConditionTrafficReady)
		if traffic == nil || traffic.Status != wantReady {
			t.Fatalf("unexpected traffic condition: %#v", traffic)
		}
	}

	run(t, func(context.Context, *operatorv1alpha1.K1sExposure) error { return nil }, metav1.ConditionTrue)
	run(t, func(context.Context, *operatorv1alpha1.K1sExposure) error { return fmt.Errorf("HTTP 503") }, metav1.ConditionFalse)
}
