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
