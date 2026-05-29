package main

import (
	"reflect"
	"testing"
)

func TestWatchNamespaces(t *testing.T) {
	got := watchNamespaces(" apps , k1s-dev-a ,,")
	want := []string{"apps", "k1s-dev-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected namespaces: got %#v want %#v", got, want)
	}
	if got := watchNamespaces(" "); len(got) != 0 {
		t.Fatalf("expected empty namespace list, got %#v", got)
	}
}

func TestCSVValuesDeduplicatesAndTrims(t *testing.T) {
	got := csvValues("Deployment, InferenceCell,Deployment,,")
	want := []string{"Deployment", "InferenceCell"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected values: got %#v want %#v", got, want)
	}
}

func TestMetricsDefaultIsLocalhost(t *testing.T) {
	if defaultMetricsBindAddress != "127.0.0.1:8080" {
		t.Fatalf("metrics should bind localhost by default, got %q", defaultMetricsBindAddress)
	}
}
