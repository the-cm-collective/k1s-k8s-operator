package controller

import (
	"testing"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildExposureResources(t *testing.T) {
	cluster := &operatorv1alpha1.K1sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-a"},
		Spec: operatorv1alpha1.K1sClusterSpec{
			Proxy: operatorv1alpha1.K1sProxySpec{
				Mode: operatorv1alpha1.K1sProxyModeService,
				ServiceRef: &operatorv1alpha1.K1sProxyServiceRef{
					Name: "k1s-edge-proxy",
					Port: 10080,
				},
			},
		},
	}
	exposure := &operatorv1alpha1.K1sExposure{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
		Spec: operatorv1alpha1.K1sExposureSpec{
			AppRef: operatorv1alpha1.K1sAppRef{Name: "echo"},
			Host:   "echo.example.test",
		},
	}
	service := BuildExposureService(cluster, exposure)
	if service.Name != "echo-k1s" || service.Spec.Selector != nil {
		t.Fatalf("unexpected service: %#v", service)
	}
	slice := BuildExposureEndpointSlice(cluster, exposure, []ProxyEndpoint{{Addresses: []string{"10.0.0.2"}, Ready: true}})
	if slice.Labels["kubernetes.io/service-name"] != service.Name || len(slice.Endpoints) != 1 {
		t.Fatalf("unexpected endpoint slice: %#v", slice)
	}
	if slice.Ports[0].Port == nil || *slice.Ports[0].Port != 10080 {
		t.Fatalf("endpoint slice should target proxy port 10080: %#v", slice.Ports)
	}
	ingress, err := BuildExposureIngress(cluster, exposure)
	if err != nil {
		t.Fatal(err)
	}
	if ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name != service.Name {
		t.Fatalf("unexpected ingress backend: %#v", ingress.Spec.Rules[0].HTTP.Paths[0].Backend)
	}
}

func TestBuildExposureIngressRequiresHostWhenEnabled(t *testing.T) {
	cluster := &operatorv1alpha1.K1sCluster{ObjectMeta: metav1.ObjectMeta{Name: "dev-a"}}
	exposure := &operatorv1alpha1.K1sExposure{ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"}}
	if _, err := BuildExposureIngress(cluster, exposure); err == nil {
		t.Fatalf("expected missing host error")
	}
}
