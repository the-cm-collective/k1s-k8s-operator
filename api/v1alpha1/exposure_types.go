package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type K1sExposureServiceSpec struct {
	Name string `json:"name,omitempty"`
	Port int32  `json:"port,omitempty"`
}

type K1sExposureIngressSpec struct {
	Enabled       *bool             `json:"enabled,omitempty"`
	Name          string            `json:"name,omitempty"`
	ClassName     *string           `json:"className,omitempty"`
	TLSSecretName string            `json:"tlsSecretName,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

func (s K1sExposureIngressSpec) EnabledOrDefault() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

type K1sExposureSpec struct {
	ClusterRef NamespacedNameRef      `json:"clusterRef"`
	AppRef     K1sAppRef              `json:"appRef"`
	Host       string                 `json:"host,omitempty"`
	Path       string                 `json:"path,omitempty"`
	Service    K1sExposureServiceSpec `json:"service,omitempty"`
	Ingress    K1sExposureIngressSpec `json:"ingress,omitempty"`
}

type K1sExposureStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	AppReady           bool               `json:"appReady,omitempty"`
	ServiceName        string             `json:"serviceName,omitempty"`
	EndpointSliceName  string             `json:"endpointSliceName,omitempty"`
	IngressName        string             `json:"ingressName,omitempty"`
	ProxyEndpointCount int32              `json:"proxyEndpointCount,omitempty"`
	LastAppSyncTime    *metav1.Time       `json:"lastAppSyncTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k1se
type K1sExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              K1sExposureSpec   `json:"spec,omitempty"`
	Status            K1sExposureStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type K1sExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K1sExposure `json:"items"`
}
