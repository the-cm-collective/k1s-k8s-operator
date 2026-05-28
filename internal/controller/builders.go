package controller

import (
	"fmt"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ProxyEndpoint struct {
	Addresses []string
	NodeName  *string
	Ready     bool
}

func BuildExposureService(cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure) *corev1.Service {
	port := exposureServicePort(exposure)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      exposureServiceName(exposure),
			Namespace: exposure.Namespace,
			Labels:    managedLabels(cluster.Name, exposure),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Protocol:   corev1.ProtocolTCP,
				Port:       port,
				TargetPort: intstr.FromInt32(port),
			}},
		},
	}
}

func BuildExposureEndpointSlice(cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure, endpoints []ProxyEndpoint) *discoveryv1.EndpointSlice {
	port := proxyTargetPort(cluster, exposure)
	protocol := corev1.ProtocolTCP
	labels := managedLabels(cluster.Name, exposure)
	labels[discoveryv1.LabelServiceName] = exposureServiceName(exposure)
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      exposureEndpointSliceName(exposure),
			Namespace: exposure.Namespace,
			Labels:    labels,
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports: []discoveryv1.EndpointPort{{
			Name:     ptr("http"),
			Protocol: &protocol,
			Port:     &port,
		}},
	}
	for _, endpoint := range endpoints {
		if len(endpoint.Addresses) == 0 {
			continue
		}
		ready := endpoint.Ready
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Addresses:  endpoint.Addresses,
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			NodeName:   endpoint.NodeName,
		})
	}
	return slice
}

func BuildExposureIngress(cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure) (*networkingv1.Ingress, error) {
	if !exposure.Spec.Ingress.EnabledOrDefault() {
		return nil, nil
	}
	if exposure.Spec.Host == "" {
		return nil, fmt.Errorf("host is required when ingress is enabled")
	}
	pathType := networkingv1.PathTypePrefix
	port := exposureServicePort(exposure)
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        exposureIngressName(exposure),
			Namespace:   exposure.Namespace,
			Labels:      managedLabels(cluster.Name, exposure),
			Annotations: exposure.Spec.Ingress.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: exposure.Spec.Ingress.ClassName,
			Rules: []networkingv1.IngressRule{{
				Host: exposure.Spec.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     exposurePath(exposure),
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: exposureServiceName(exposure),
									Port: networkingv1.ServiceBackendPort{Number: port},
								},
							},
						}},
					},
				},
			}},
		},
	}
	if exposure.Spec.Ingress.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{{
			Hosts:      []string{exposure.Spec.Host},
			SecretName: exposure.Spec.Ingress.TLSSecretName,
		}}
	}
	return ingress, nil
}

func ptr[T any](v T) *T {
	return &v
}

func proxyTargetPort(cluster *operatorv1alpha1.K1sCluster, exposure *operatorv1alpha1.K1sExposure) int32 {
	switch cluster.Spec.Proxy.Mode {
	case operatorv1alpha1.K1sProxyModeService:
		if cluster.Spec.Proxy.ServiceRef != nil && cluster.Spec.Proxy.ServiceRef.Port > 0 {
			return cluster.Spec.Proxy.ServiceRef.Port
		}
	case operatorv1alpha1.K1sProxyModeExternal:
		if cluster.Spec.Proxy.External != nil && cluster.Spec.Proxy.External.Port > 0 {
			return cluster.Spec.Proxy.External.Port
		}
	}
	return exposureServicePort(exposure)
}
