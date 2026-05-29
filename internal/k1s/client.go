package k1s

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	leaderServiceFallbackAttempts = 5
	leaderRetryBackoff            = 200 * time.Millisecond
)

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

type Option func(*clientConfig)

type clientConfig struct {
	token    string
	caBundle []byte
	timeout  time.Duration
}

func WithBearerToken(token string) Option {
	return func(c *clientConfig) {
		c.token = strings.TrimSpace(token)
	}
}

func WithCABundle(bundle []byte) Option {
	return func(c *clientConfig) {
		c.caBundle = append([]byte(nil), bundle...)
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *clientConfig) {
		c.timeout = timeout
	}
}

func NewClient(rawBaseURL string, opts ...Option) (*Client, error) {
	base, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return nil, err
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("k1s base URL must include scheme and host")
	}
	cfg := clientConfig{timeout: 10 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if len(cfg.caBundle) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.caBundle) {
			return nil, fmt.Errorf("invalid CA bundle")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &Client{
		baseURL: base,
		token:   cfg.token,
		http:    &http.Client{Transport: transport, Timeout: cfg.timeout},
	}, nil
}

func (c *Client) Health(ctx context.Context) error {
	var payload map[string]any
	err := c.getJSON(ctx, "/health", &payload)
	if err == nil {
		return nil
	}
	return c.getJSON(ctx, "/healthz", &payload)
}

func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	var payload map[string]any
	err := c.getJSON(ctx, "/status", &payload)
	return payload, err
}

func (c *Client) AppStatus(ctx context.Context, namespace, name string) (AppStatus, error) {
	appRef := name
	if namespace != "" && namespace != "default" {
		appRef = namespace + "/" + name
	}
	var payload map[string]any
	err := c.getJSON(ctx, "/status/"+url.PathEscape(appRef)+"?details=1", &payload)
	if err != nil && strings.Contains(appRef, "/") {
		err = c.getJSON(ctx, "/status/"+strings.ReplaceAll(appRef, "/", "--")+"?details=1", &payload)
	}
	if err != nil {
		return AppStatus{}, err
	}
	return ParseAppStatus(payload), nil
}

func (c *Client) Nodes(ctx context.Context) (NodeSummary, error) {
	var payload map[string]any
	err := c.getJSON(ctx, "/nodes", &payload)
	if err != nil {
		return NodeSummary{}, err
	}
	return ParseNodeSummary(payload), nil
}

func (c *Client) Apply(ctx context.Context, manifest []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(manifest, &payload); err != nil {
		return nil, err
	}
	var out map[string]any
	err := c.postJSON(ctx, "/apply", payload, &out)
	return out, err
}

func (c *Client) DeleteApp(ctx context.Context, app string, purge bool) (map[string]any, error) {
	endpoint := "/delete/" + url.PathEscape(app)
	if purge {
		endpoint += "?purge=1"
	}
	var out map[string]any
	err := c.postJSON(ctx, endpoint, map[string]any{}, &out)
	return out, err
}

func (c *Client) InferenceCellStatus(ctx context.Context, namespace, name string) (InferenceStatus, error) {
	return c.inferenceStatus(ctx, "cells", namespace, name)
}

func (c *Client) InferenceCellSetStatus(ctx context.Context, namespace, name string) (InferenceStatus, error) {
	return c.inferenceStatus(ctx, "cellsets", namespace, name)
}

func (c *Client) DeleteInferenceCell(ctx context.Context, namespace, name string) (map[string]any, error) {
	return c.deleteInference(ctx, "cells", namespace, name)
}

func (c *Client) DeleteInferenceCellSet(ctx context.Context, namespace, name string) (map[string]any, error) {
	return c.deleteInference(ctx, "cellsets", namespace, name)
}

func (c *Client) inferenceStatus(ctx context.Context, collection, namespace, name string) (InferenceStatus, error) {
	endpoint := "/inference/" + collection + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(name)
	var payload map[string]any
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return InferenceStatus{}, err
	}
	return ParseInferenceStatus(payload), nil
}

func (c *Client) deleteInference(ctx context.Context, collection, namespace, name string) (map[string]any, error) {
	endpoint := "/inference/delete/" + collection + "/" + url.PathEscape(name)
	if namespace != "" {
		endpoint += "?namespace=" + url.QueryEscape(namespace)
	}
	var out map[string]any
	err := c.postJSON(ctx, endpoint, map[string]any{}, &out)
	return out, err
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	return c.doJSONWithLeaderRetry(ctx, http.MethodGet, endpoint, nil, out)
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.doJSONWithLeaderRetry(ctx, http.MethodPost, endpoint, body, out)
}

func (c *Client) doJSONWithLeaderRetry(ctx context.Context, method, endpoint string, body []byte, out any) error {
	originalURL := c.url(endpoint)
	err := c.doJSONRequest(ctx, method, originalURL, body, out)
	redirect, ok := err.(*notLeaderError)
	if !ok {
		return err
	}

	var leaderBase *url.URL
	var leaderErr error
	if redirect.advertiseAddr != "" {
		var parseErr error
		leaderBase, parseErr = url.Parse(redirect.advertiseAddr)
		if parseErr != nil {
			return err
		}
		leaderURL := buildURL(leaderBase, endpoint)
		leaderErr = c.doJSONRequest(ctx, method, leaderURL, body, out)
		if leaderErr == nil {
			return nil
		}
	}

	fallbackErr := err
	for attempt := 0; attempt < leaderServiceFallbackAttempts; attempt++ {
		if err := sleepContext(ctx, leaderRetryBackoff); err != nil {
			return err
		}
		fallbackErr = c.doJSONRequest(ctx, method, originalURL, body, out)
		if fallbackErr == nil {
			return nil
		}
		if _, stillFollower := fallbackErr.(*notLeaderError); !stillFollower {
			break
		}
	}

	if leaderErr != nil {
		return fmt.Errorf("k1s leader retry failed after %d service fallbacks: advertised %s: %w; service %s: %v", leaderServiceFallbackAttempts, leaderBase.Redacted(), leaderErr, c.baseURL.Redacted(), fallbackErr)
	}
	return fmt.Errorf("k1s leader retry failed after %d service fallbacks against %s: %w", leaderServiceFallbackAttempts, c.baseURL.Redacted(), fallbackErr)
}

func (c *Client) doJSONRequest(ctx context.Context, method, rawURL string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if redirect := parseNotLeader(resp.StatusCode, respBody); redirect != nil {
			redirect.method = req.Method
			redirect.path = req.URL.Path
			redirect.body = strings.TrimSpace(string(respBody))
			return redirect
		}
		return &APIError{Method: req.Method, Path: req.URL.Path, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("k1s API %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func (c *Client) url(endpoint string) string {
	return buildURL(c.baseURL, endpoint)
}

func buildURL(base *url.URL, endpoint string) string {
	next := *base
	basePath := strings.TrimRight(next.Path, "/")
	endpointPath := "/" + strings.TrimLeft(endpoint, "/")
	next.Path = path.Join(basePath, endpointPath)
	if strings.HasSuffix(endpointPath, "/") && !strings.HasSuffix(next.Path, "/") {
		next.Path += "/"
	}
	if strings.Contains(endpoint, "?") {
		parts := strings.SplitN(endpoint, "?", 2)
		next.Path = path.Join(basePath, "/"+strings.TrimLeft(parts[0], "/"))
		next.RawQuery = parts[1]
	}
	return next.String()
}

type notLeaderError struct {
	advertiseAddr string
	method        string
	path          string
	body          string
}

func (e *notLeaderError) Error() string {
	return fmt.Sprintf("k1s API %s %s returned 409: %s", e.method, e.path, e.body)
}

func parseNotLeader(statusCode int, body []byte) *notLeaderError {
	if statusCode != http.StatusConflict {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if strv(payload["error"]) != "not_leader" {
		return nil
	}
	return &notLeaderError{advertiseAddr: strv(payload["advertise_addr"])}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func strv(v any) string {
	if value, ok := v.(string); ok {
		return value
	}
	return ""
}
