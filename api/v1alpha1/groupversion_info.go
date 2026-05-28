// Package v1alpha1 contains API schema definitions for the k1s operator.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const GroupName = "operator.k1s.io"

var (
	GroupVersion  = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(
		&K1sCluster{},
		&K1sClusterList{},
		&K1sExposure{},
		&K1sExposureList{},
		&K1sAppMirror{},
		&K1sAppMirrorList{},
		&K1sApp{},
		&K1sAppList{},
		&K1sInferenceEndpoint{},
		&K1sInferenceEndpointList{},
		&K1sResourceSet{},
		&K1sResourceSetList{},
	)
}

func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = AddToScheme(s)
	return s
}
