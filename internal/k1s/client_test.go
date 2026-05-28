package k1s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseAppStatusReady(t *testing.T) {
	status := ParseAppStatus(map[string]any{
		"app_name":         "demo--echo",
		"name":             "echo",
		"namespace":        "demo",
		"desired_replicas": float64(2),
		"ready_replicas":   float64(2),
		"live_replicas":    float64(2),
		"revision":         float64(7),
		"revision_status":  "ready",
		"image":            "example/echo:latest",
		"ingress_host":     "echo.example.test",
	})
	if !status.Ready {
		t.Fatalf("expected ready status")
	}
	if status.Revision != "7" {
		t.Fatalf("expected revision string 7, got %q", status.Revision)
	}
	if status.RevisionStatus != "ready" {
		t.Fatalf("expected revision status ready, got %q", status.RevisionStatus)
	}
}

func TestParseNodeSummary(t *testing.T) {
	summary := ParseNodeSummary(map[string]any{
		"nodes": []any{
			map[string]any{"status": "Ready", "stale": false},
			map[string]any{"status": "Ready", "stale": true},
			map[string]any{"status": "NotReady", "stale": false},
		},
	})
	if summary.Total != 3 || summary.Ready != 1 || summary.Stale != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestApplyRetriesAdvertisedLeader(t *testing.T) {
	var leaderHit bool
	leader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaderHit = true
		if r.URL.Path != "/apply" {
			t.Fatalf("unexpected leader path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"app":"demo","status":"ready"}`))
	}))
	defer leader.Close()

	follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"not_leader","advertise_addr":"` + leader.URL + `"}`))
	}))
	defer follower.Close()

	client, err := NewClient(follower.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Apply(context.Background(), []byte(`{"apiVersion":"ae.dev/v1alpha1","kind":"Deployment","metadata":{"name":"demo"},"spec":{"image":"busybox"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !leaderHit || result["status"] != "ready" {
		t.Fatalf("leader retry did not succeed: hit=%v result=%#v", leaderHit, result)
	}
}
