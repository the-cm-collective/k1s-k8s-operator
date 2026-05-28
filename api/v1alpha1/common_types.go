package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type NamespacedNameRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (r NamespacedNameRef) NamespaceOr(defaultNamespace string) string {
	if r.Namespace != "" {
		return r.Namespace
	}
	return defaultNamespace
}

type K1sEndpointSpec struct {
	URL string `json:"url"`
}

type K1sAuthSecretRef struct {
	Name                    string `json:"name"`
	Namespace               string `json:"namespace,omitempty"`
	ControllerReadTokenKey  string `json:"controllerReadTokenKey,omitempty"`
	ControllerWriteTokenKey string `json:"controllerWriteTokenKey,omitempty"`
	ApishimReadTokenKey     string `json:"apishimReadTokenKey,omitempty"`
	ApishimWriteTokenKey    string `json:"apishimWriteTokenKey,omitempty"`
	CABundleKey             string `json:"caBundleKey,omitempty"`
}

func (r K1sAuthSecretRef) NamespaceOr(defaultNamespace string) string {
	if r.Namespace != "" {
		return r.Namespace
	}
	return defaultNamespace
}

type K1sAppRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (r K1sAppRef) NamespaceOrDefault() string {
	if r.Namespace != "" {
		return r.Namespace
	}
	return "default"
}

type ConditionedStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type NativeManifest struct {
	Raw runtime.RawExtension `json:"raw"`
}

const (
	ConditionReady               = "Ready"
	ConditionControllerAvailable = "ControllerAvailable"
	ConditionProxyReady          = "ProxyReady"
	ConditionCredentialsValid    = "CredentialsValid"
	ConditionBootstrapLoaded     = "BootstrapLoaded"
	ConditionApishimAvailable    = "ApishimAvailable"
	ConditionClusterReady        = "ClusterReady"
	ConditionAppFound            = "AppFound"
	ConditionAppReady            = "AppReady"
	ConditionProxyEndpointsReady = "ProxyEndpointsReady"
	ConditionResourcesApplied    = "ResourcesApplied"
	ConditionIngressReady        = "IngressReady"
	ConditionAccepted            = "Accepted"
	ConditionApplied             = "Applied"
	ConditionPolicyAllowed       = "PolicyAllowed"
)

const (
	ReasonReady       = "Ready"
	ReasonUnavailable = "Unavailable"
	ReasonMissing     = "Missing"
	ReasonInvalid     = "Invalid"
	ReasonApplied     = "Applied"
	ReasonPending     = "Pending"
	ReasonUnsupported = "Unsupported"
	ReasonDenied      = "Denied"
)
