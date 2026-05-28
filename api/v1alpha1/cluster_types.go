package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type K1sProxyMode string

const (
	K1sProxyModeService  K1sProxyMode = "Service"
	K1sProxyModeExternal K1sProxyMode = "External"
)

type K1sProxyServiceRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Port      int32  `json:"port"`
}

type K1sProxyExternalSpec struct {
	Addresses []string `json:"addresses,omitempty"`
	DNSName   string   `json:"dnsName,omitempty"`
	Port      int32    `json:"port"`
}

type K1sProxySpec struct {
	Mode       K1sProxyMode          `json:"mode"`
	ServiceRef *K1sProxyServiceRef   `json:"serviceRef,omitempty"`
	External   *K1sProxyExternalSpec `json:"external,omitempty"`
}

type K1sClusterSpec struct {
	Controller            K1sEndpointSpec    `json:"controller"`
	Apishim               *K1sEndpointSpec   `json:"apishim,omitempty"`
	BootstrapConfigMapRef *NamespacedNameRef `json:"bootstrapConfigMapRef,omitempty"`
	AuthSecretRef         *K1sAuthSecretRef  `json:"authSecretRef,omitempty"`
	Proxy                 K1sProxySpec       `json:"proxy"`
	PollIntervalSeconds   *int32             `json:"pollIntervalSeconds,omitempty"`
}

type K1sNodeSummary struct {
	Ready int32 `json:"ready,omitempty"`
	Stale int32 `json:"stale,omitempty"`
	Total int32 `json:"total,omitempty"`
}

type K1sClusterStatus struct {
	ObservedGeneration  int64              `json:"observedGeneration,omitempty"`
	ControllerAvailable bool               `json:"controllerAvailable,omitempty"`
	ApishimAvailable    bool               `json:"apishimAvailable,omitempty"`
	ProxyReady          bool               `json:"proxyReady,omitempty"`
	StackDomain         string             `json:"stackDomain,omitempty"`
	WildcardAppsDomain  string             `json:"wildcardAppsDomain,omitempty"`
	NodeSummary         K1sNodeSummary     `json:"nodeSummary,omitempty"`
	Conditions          []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k1sc
type K1sCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              K1sClusterSpec   `json:"spec,omitempty"`
	Status            K1sClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type K1sClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K1sCluster `json:"items"`
}
