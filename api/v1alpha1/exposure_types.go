package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type K1sExposureServiceSpec struct {
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
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

type K1sTrafficProbeType string

const (
	K1sTrafficProbeTypeHTTP K1sTrafficProbeType = "HTTP"
	K1sTrafficProbeTypeTCP  K1sTrafficProbeType = "TCP"
)

type K1sTrafficProbeScheme string

const (
	K1sTrafficProbeSchemeHTTP  K1sTrafficProbeScheme = "HTTP"
	K1sTrafficProbeSchemeHTTPS K1sTrafficProbeScheme = "HTTPS"
)

type K1sExposureTrafficProbeSpec struct {
	Enabled *bool `json:"enabled,omitempty"`
	// +kubebuilder:validation:Enum=HTTP;TCP
	Type K1sTrafficProbeType `json:"type,omitempty"`
	// +kubebuilder:validation:Enum=HTTP;HTTPS
	Scheme K1sTrafficProbeScheme `json:"scheme,omitempty"`
	Path   string                `json:"path,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
	// +kubebuilder:validation:Pattern=`^[1-5][0-9][0-9](-[1-5][0-9][0-9])?$`
	ExpectedStatus string `json:"expectedStatus,omitempty"`
}

func (s K1sExposureTrafficProbeSpec) EnabledOrDefault() bool {
	return s.Enabled != nil && *s.Enabled
}

// +kubebuilder:validation:XValidation:rule="!has(self.ingress) ? (has(self.host) && size(self.host) > 0) : ((!has(self.ingress.enabled) || self.ingress.enabled) ? (has(self.host) && size(self.host) > 0) : true)",message="host is required when ingress is enabled"
type K1sExposureSpec struct {
	ClusterRef   NamespacedNameRef           `json:"clusterRef"`
	AppRef       K1sAppRef                   `json:"appRef"`
	Host         string                      `json:"host,omitempty"`
	Path         string                      `json:"path,omitempty"`
	Service      K1sExposureServiceSpec      `json:"service,omitempty"`
	Ingress      K1sExposureIngressSpec      `json:"ingress,omitempty"`
	TrafficProbe K1sExposureTrafficProbeSpec `json:"trafficProbe,omitempty"`
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
