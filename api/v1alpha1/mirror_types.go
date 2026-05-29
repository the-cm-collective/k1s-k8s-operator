package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type K1sAppMirrorSpec struct {
	ClusterRef  NamespacedNameRef  `json:"clusterRef"`
	AppRef      K1sAppRef          `json:"appRef"`
	ExposureRef *NamespacedNameRef `json:"exposureRef,omitempty"`
}

type K1sReplicaSummary struct {
	Desired int32 `json:"desired"`
	Ready   int32 `json:"ready"`
	Live    int32 `json:"live"`
}

type K1sPlacementStatus struct {
	Node  string `json:"node,omitempty"`
	Site  string `json:"site,omitempty"`
	Ready bool   `json:"ready"`
}

type K1sAppMirrorStatus struct {
	ObservedGeneration int64                `json:"observedGeneration,omitempty"`
	Ready              bool                 `json:"ready"`
	Replicas           K1sReplicaSummary    `json:"replicas,omitempty"`
	Image              string               `json:"image,omitempty"`
	Revision           string               `json:"revision,omitempty"`
	ObservedHost       string               `json:"observedHost,omitempty"`
	Placements         []K1sPlacementStatus `json:"placements,omitempty"`
	LastSyncTime       *metav1.Time         `json:"lastSyncTime,omitempty"`
	Conditions         []metav1.Condition   `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=k1sm
type K1sAppMirror struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              K1sAppMirrorSpec   `json:"spec,omitempty"`
	Status            K1sAppMirrorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type K1sAppMirrorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []K1sAppMirror `json:"items"`
}
