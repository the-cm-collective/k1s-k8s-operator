package controller

import (
	"encoding/json"
	"testing"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildInferenceManifestSingleCell(t *testing.T) {
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "llama", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sInferenceEndpointSpec{
			Model:   operatorv1alpha1.K1sInferenceModelSpec{ModelID: "llama-3.1-8b", LocalPath: "/models/llama"},
			Members: []operatorv1alpha1.K1sInferenceMemberSpec{{SiteID: "edge-a", NodeID: "gpu-1", GPUCount: 1}},
		},
	}
	raw, err := buildInferenceManifest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "InferenceCell" {
		t.Fatalf("unexpected kind: %v", payload["kind"])
	}
	spec := payload["spec"].(map[string]any)
	model := spec["model"].(map[string]any)
	if model["modelId"] != "llama-3.1-8b" {
		t.Fatalf("unexpected model: %#v", model)
	}
}

func TestBuildInferenceManifestCellSet(t *testing.T) {
	endpoint := &operatorv1alpha1.K1sInferenceEndpoint{
		ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: "ml"},
		Spec: operatorv1alpha1.K1sInferenceEndpointSpec{
			Model:   operatorv1alpha1.K1sInferenceModelSpec{ModelID: "model"},
			CellSet: &operatorv1alpha1.K1sInferenceCellSetSpec{Replicas: 2},
		},
	}
	raw, err := buildInferenceManifest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if kindFromManifest(raw) != "InferenceCellSet" {
		t.Fatalf("expected InferenceCellSet manifest: %s", raw)
	}
}
