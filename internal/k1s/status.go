package k1s

import "strconv"

type AppStatus struct {
	AppName         string
	Name            string
	Namespace       string
	Ready           bool
	DesiredReplicas int32
	ReadyReplicas   int32
	LiveReplicas    int32
	Revision        string
	RevisionStatus  string
	Image           string
	IngressHost     string
	IngressPath     string
	Placements      []Placement
}

type Placement struct {
	Node  string
	Ready bool
}

type NodeSummary struct {
	Ready int32
	Stale int32
	Total int32
}

type InferenceStatus struct {
	Kind           string
	Name           string
	Namespace      string
	Phase          string
	Status         string
	Ready          bool
	APIEndpoint    string
	ActiveExecutor string
	LastError      string
	Desired        int32
	Current        int32
	ReadyCount     int32
}

func ParseAppStatus(payload map[string]any) AppStatus {
	status := AppStatus{
		AppName:         str(payload["app_name"]),
		Name:            str(payload["name"]),
		Namespace:       str(payload["namespace"]),
		DesiredReplicas: i32(payload["desired_replicas"]),
		ReadyReplicas:   i32(payload["ready_replicas"]),
		LiveReplicas:    i32(payload["live_replicas"]),
		Revision:        str(payload["revision"]),
		RevisionStatus:  str(payload["revision_status"]),
		Image:           str(payload["image"]),
		IngressHost:     str(payload["ingress_host"]),
		IngressPath:     str(payload["ingress_path"]),
	}
	status.Ready = status.DesiredReplicas > 0 && status.ReadyReplicas >= status.DesiredReplicas
	if rawPods, ok := payload["pods"].([]any); ok {
		for _, item := range rawPods {
			pod, ok := item.(map[string]any)
			if !ok {
				continue
			}
			status.Placements = append(status.Placements, Placement{
				Node:  str(pod["node"]),
				Ready: boolv(pod["ready"]),
			})
		}
	}
	return status
}

func ParseInferenceStatus(payload map[string]any) InferenceStatus {
	phase := firstStatus(str(payload["phase"]), str(payload["status"]))
	status := InferenceStatus{
		Kind:           str(payload["kind"]),
		Name:           str(payload["name"]),
		Namespace:      str(payload["namespace"]),
		Phase:          phase,
		Status:         str(payload["status"]),
		Ready:          boolv(payload["ready"]) || equalFoldReady(phase),
		APIEndpoint:    str(payload["api_endpoint"]),
		ActiveExecutor: str(payload["active_executor"]),
		LastError:      str(payload["last_error"]),
		Desired:        i32(payload["desired"]),
		Current:        i32(payload["current"]),
		ReadyCount:     i32(payload["ready"]),
	}
	if status.Kind == "InferenceCellSet" && status.Desired > 0 {
		status.Ready = status.Ready || status.ReadyCount >= status.Desired
	}
	return status
}

func ParseNodeSummary(payload map[string]any) NodeSummary {
	summary := NodeSummary{Total: i32(payload["count"])}
	nodes, ok := payload["nodes"].([]any)
	if !ok {
		return summary
	}
	summary.Total = int32(len(nodes))
	for _, item := range nodes {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if boolv(node["stale"]) {
			summary.Stale++
		}
		if str(node["status"]) == "Ready" && !boolv(node["stale"]) {
			summary.Ready++
		}
	}
	return summary
}

func firstStatus(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func equalFoldReady(value string) bool {
	return value == "ready" || value == "Ready" || value == "READY"
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return ""
	}
}

func i32(v any) int32 {
	switch t := v.(type) {
	case float64:
		return int32(t)
	case int:
		return int32(t)
	case int32:
		return t
	case int64:
		return int32(t)
	case string:
		i, _ := strconv.Atoi(t)
		return int32(i)
	default:
		return 0
	}
}

func boolv(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "True" || t == "READY" || t == "Ready"
	default:
		return false
	}
}
