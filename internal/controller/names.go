package controller

import (
	"strings"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
)

const (
	labelName         = "app.kubernetes.io/name"
	labelManagedBy    = "app.kubernetes.io/managed-by"
	labelCluster      = "operator.k1s.io/cluster"
	labelExposure     = "operator.k1s.io/exposure"
	labelAppNamespace = "operator.k1s.io/app-namespace"
	labelAppName      = "operator.k1s.io/app-name"
)

func exposureServiceName(exposure *operatorv1alpha1.K1sExposure) string {
	if exposure.Spec.Service.Name != "" {
		return exposure.Spec.Service.Name
	}
	return dnsLabel(exposure.Name + "-k1s")
}

func exposureEndpointSliceName(exposure *operatorv1alpha1.K1sExposure) string {
	return dnsLabel(exposureServiceName(exposure) + "-proxy")
}

func exposureIngressName(exposure *operatorv1alpha1.K1sExposure) string {
	if exposure.Spec.Ingress.Name != "" {
		return exposure.Spec.Ingress.Name
	}
	return dnsLabel(exposure.Name + "-k1s")
}

func exposurePath(exposure *operatorv1alpha1.K1sExposure) string {
	if exposure.Spec.Path != "" {
		return exposure.Spec.Path
	}
	return "/"
}

func exposureServicePort(exposure *operatorv1alpha1.K1sExposure) int32 {
	if exposure.Spec.Service.Port > 0 {
		return exposure.Spec.Service.Port
	}
	return 80
}

func dnsLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.Trim(value, "-.")
	if len(value) > 63 {
		value = strings.TrimRight(value[:63], "-")
	}
	if value == "" {
		return "k1s"
	}
	return value
}

func managedLabels(clusterName string, exposure *operatorv1alpha1.K1sExposure) map[string]string {
	return map[string]string{
		labelName:         "k1s-operator",
		labelManagedBy:    "k1s-operator",
		labelCluster:      clusterName,
		labelExposure:     exposure.Name,
		labelAppNamespace: exposure.Spec.AppRef.NamespaceOrDefault(),
		labelAppName:      exposure.Spec.AppRef.Name,
	}
}
