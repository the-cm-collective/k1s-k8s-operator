package controller

import (
	"context"
	"fmt"
	"net"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func discoverProxyEndpoints(ctx context.Context, kube client.Client, cluster *operatorv1alpha1.K1sCluster) ([]ProxyEndpoint, error) {
	switch cluster.Spec.Proxy.Mode {
	case operatorv1alpha1.K1sProxyModeService:
		if cluster.Spec.Proxy.ServiceRef == nil {
			return nil, fmt.Errorf("proxy.serviceRef is required when proxy.mode=Service")
		}
		ref := cluster.Spec.Proxy.ServiceRef
		namespace := ref.Namespace
		if namespace == "" {
			namespace = cluster.Namespace
		}
		var slices discoveryv1.EndpointSliceList
		selector := labels.SelectorFromSet(map[string]string{discoveryv1.LabelServiceName: ref.Name})
		if err := kube.List(ctx, &slices, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return nil, err
		}
		var out []ProxyEndpoint
		for _, slice := range slices.Items {
			for _, endpoint := range slice.Endpoints {
				ready := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
				if !ready {
					continue
				}
				out = append(out, ProxyEndpoint{
					Addresses: append([]string(nil), endpoint.Addresses...),
					NodeName:  endpoint.NodeName,
					Ready:     true,
				})
			}
		}
		return out, nil
	case operatorv1alpha1.K1sProxyModeExternal:
		if cluster.Spec.Proxy.External == nil {
			return nil, fmt.Errorf("proxy.external is required when proxy.mode=External")
		}
		external := cluster.Spec.Proxy.External
		var out []ProxyEndpoint
		for _, address := range external.Addresses {
			if net.ParseIP(address) != nil {
				out = append(out, ProxyEndpoint{Addresses: []string{address}, Ready: true})
			}
		}
		if external.DNSName != "" {
			addrs, err := net.DefaultResolver.LookupHost(ctx, external.DNSName)
			if err != nil {
				return out, err
			}
			for _, address := range addrs {
				out = append(out, ProxyEndpoint{Addresses: []string{address}, Ready: true})
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported proxy.mode %q", cluster.Spec.Proxy.Mode)
	}
}
