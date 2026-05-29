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

func TestParseInferenceStatusReady(t *testing.T) {
	status := ParseInferenceStatus(map[string]any{
		"kind":            "InferenceCell",
		"name":            "llama",
		"namespace":       "ml",
		"phase":           "READY",
		"status":          "ready",
		"ready":           true,
		"api_endpoint":    "10.0.0.10:18080",
		"active_executor": "ray",
	})
	if !status.Ready || status.Phase != "READY" || status.APIEndpoint == "" || status.ActiveExecutor != "ray" {
		t.Fatalf("unexpected inference status: %#v", status)
	}
}

func TestInferenceStatusAndDeleteUseNativeRoutes(t *testing.T) {
	var sawDelete bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference/cells/ml/llama":
			_, _ = w.Write([]byte(`{"kind":"InferenceCell","name":"llama","namespace":"ml","phase":"READY","ready":true}`))
		case "/inference/delete/cells/llama":
			sawDelete = r.URL.Query().Get("namespace") == "ml"
			_, _ = w.Write([]byte(`{"kind":"InferenceCell","name":"llama","namespace":"ml","removed":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.InferenceCellStatus(context.Background(), "ml", "llama")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.Name != "llama" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if _, err := client.DeleteInferenceCell(context.Background(), "ml", "llama"); err != nil {
		t.Fatal(err)
	}
	if !sawDelete {
		t.Fatalf("delete route did not include namespace query")
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

func TestApplyFallsBackToServiceURLWhenAdvertisedLeaderUnreachable(t *testing.T) {
	var hits int
	follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits == 1 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"not_leader","advertise_addr":"http://127.0.0.1:1"}`))
			return
		}
		if r.URL.Path != "/apply" {
			t.Fatalf("unexpected fallback path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"app":"demo","status":"ready"}`))
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
	if hits != 2 || result["status"] != "ready" {
		t.Fatalf("expected one fallback retry, hits=%d result=%#v", hits, result)
	}
}

func TestApplyKeepsFallbackToServiceURLAcrossFollowers(t *testing.T) {
	var hits int
	follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits < 4 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"not_leader","advertise_addr":"http://127.0.0.1:1"}`))
			return
		}
		if r.URL.Path != "/apply" {
			t.Fatalf("unexpected fallback path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"app":"demo","status":"ready"}`))
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
	if hits != 4 || result["status"] != "ready" {
		t.Fatalf("expected repeated service fallbacks, hits=%d result=%#v", hits, result)
	}
}

func TestRequestsUseOneShotConnections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.Close {
			t.Fatalf("expected k1s client request to close the connection")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AppStatus(context.Background(), "default", "missing")
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound for 404, got %v", err)
	}
}
