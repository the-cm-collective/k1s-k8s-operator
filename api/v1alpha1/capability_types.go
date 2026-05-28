package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type K1sDeletePolicy string

const (
	K1sDeletePolicyDelete K1sDeletePolicy = "Delete"
	K1sDeletePolicyOrphan K1sDeletePolicy = "Orphan"
)

type K1sAppSpec struct {
	ClusterRef   NamespacedNameRef    `json:"clusterRef"`
	Manifest     runtime.RawExtension `json:"manifest"`
	DeletePolicy K1sDeletePolicy      `json:"deletePolicy,omitempty"`
}

type K1sAppStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	AppName            string             `json:"appName,omitempty"`
	Ready              bool               `json:"ready,omitempty"`
	Phase              string             `json:"phase,omitempty"`
	Endpoint           string             `json:"endpoint,omitempty"`
	Replicas           K1sReplicaSummary  `json:"replicas,omitempty"`
	Image              string             `json:"image,omitempty"`
	Revision           string             `json:"revision,omitempty"`
	LastSyncTime       *metav1.Time       `json:"lastSyncTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k1sa
type K1sApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              K1sAppSpec   `json:"spec,omitempty"`
	Status            K1sAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type K1sAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K1sApp `json:"items"`
}

type K1sInferenceModelSpec struct {
	ModelID   string `json:"modelId"`
	LocalPath string `json:"localPath,omitempty"`
}

type K1sInferenceParallelismSpec struct {
	TP int32 `json:"tp,omitempty"`
	PP int32 `json:"pp,omitempty"`
}

type K1sInferenceExecutorSpec struct {
	Type             string `json:"type,omitempty"`
	FallbackMode     string `json:"fallbackMode,omitempty"`
	RayImage         string `json:"rayImage,omitempty"`
	MPImage          string `json:"mpImage,omitempty"`
	LauncherImage    string `json:"launcherImage,omitempty"`
	DType            string `json:"dtype,omitempty"`
	RuntimeClassName string `json:"runtimeClassName,omitempty"`
}

type K1sInferenceMemberSpec struct {
	SiteID   string `json:"siteId,omitempty"`
	NodeID   string `json:"nodeId,omitempty"`
	GPUCount int32  `json:"gpuCount,omitempty"`
}

type K1sInferenceFabricSpec struct {
	Mode       string `json:"mode,omitempty"`
	PolicyMode string `json:"policyMode,omitempty"`
	TTLSeconds int32  `json:"ttlSeconds,omitempty"`
}

type K1sInferenceCellSetSpec struct {
	Replicas   int32  `json:"replicas,omitempty"`
	NameFormat string `json:"nameFormat,omitempty"`
}

type K1sInferenceEndpointSpec struct {
	ClusterRef   NamespacedNameRef           `json:"clusterRef"`
	Model        K1sInferenceModelSpec       `json:"model"`
	Parallelism  K1sInferenceParallelismSpec `json:"parallelism,omitempty"`
	Executor     K1sInferenceExecutorSpec    `json:"executor,omitempty"`
	Fabric       K1sInferenceFabricSpec      `json:"fabric,omitempty"`
	Members      []K1sInferenceMemberSpec    `json:"members,omitempty"`
	CellSet      *K1sInferenceCellSetSpec    `json:"cellSet,omitempty"`
	DeletePolicy K1sDeletePolicy             `json:"deletePolicy,omitempty"`
}

type K1sInferenceEndpointStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Ready              bool               `json:"ready,omitempty"`
	Phase              string             `json:"phase,omitempty"`
	CellName           string             `json:"cellName,omitempty"`
	CellSetName        string             `json:"cellSetName,omitempty"`
	APIEndpoint        string             `json:"apiEndpoint,omitempty"`
	ActiveExecutor     string             `json:"activeExecutor,omitempty"`
	LastError          string             `json:"lastError,omitempty"`
	LastSyncTime       *metav1.Time       `json:"lastSyncTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k1sinf
type K1sInferenceEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              K1sInferenceEndpointSpec   `json:"spec,omitempty"`
	Status            K1sInferenceEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type K1sInferenceEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K1sInferenceEndpoint `json:"items"`
}

type K1sResourceSetSpec struct {
	ClusterRef   NamespacedNameRef      `json:"clusterRef"`
	Manifests    []runtime.RawExtension `json:"manifests,omitempty"`
	AllowedKinds []string               `json:"allowedKinds,omitempty"`
	Prune        bool                   `json:"prune,omitempty"`
	DeletePolicy K1sDeletePolicy        `json:"deletePolicy,omitempty"`
}

type K1sResourceSetStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Applied            int32              `json:"applied,omitempty"`
	Ready              bool               `json:"ready,omitempty"`
	LastSyncTime       *metav1.Time       `json:"lastSyncTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k1srs
type K1sResourceSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              K1sResourceSetSpec   `json:"spec,omitempty"`
	Status            K1sResourceSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type K1sResourceSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K1sResourceSet `json:"items"`
}
