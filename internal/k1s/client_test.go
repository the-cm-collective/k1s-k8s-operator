package k1s

import "testing"

func TestParseAppStatusReady(t *testing.T) {
	status := ParseAppStatus(map[string]any{
		"app_name":         "demo--echo",
		"name":             "echo",
		"namespace":        "demo",
		"desired_replicas": float64(2),
		"ready_replicas":   float64(2),
		"live_replicas":    float64(2),
		"revision":         float64(7),
		"image":            "example/echo:latest",
		"ingress_host":     "echo.example.test",
	})
	if !status.Ready {
		t.Fatalf("expected ready status")
	}
	if status.Revision != "7" {
		t.Fatalf("expected revision string 7, got %q", status.Revision)
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
