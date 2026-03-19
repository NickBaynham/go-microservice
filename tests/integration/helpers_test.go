package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// ── Config ───────────────────────────────────────────────────────────────────

type testConfig struct {
	BaseURL string
}

func loadConfig() testConfig {
	host := getEnv("TEST_HOST", "localhost")
	port := getEnv("TEST_PORT", "8443")
	scheme := getEnv("TEST_SCHEME", "https")
	return testConfig{
		BaseURL: fmt.Sprintf("%s://%s:%s", scheme, host, port),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── HTTP Client ───────────────────────────────────────────────────────────────

// newClient returns an HTTP client that skips TLS verification for self-signed certs.
// In production (TEST_SCHEME=https with real certs) set TEST_SKIP_TLS_VERIFY=false.
func newClient() *http.Client {
	skipVerify := getEnv("TEST_SKIP_TLS_VERIFY", "true") != "false"
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerify}, //nolint:gosec
		},
	}
}

// ── Request helpers ───────────────────────────────────────────────────────────

type response struct {
	StatusCode int
	Body       map[string]any
	RawBody    string
}

func do(t *testing.T, client *http.Client, method, url, token string, payload any) response {
	t.Helper()

	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)

	return response{
		StatusCode: resp.StatusCode,
		Body:       parsed,
		RawBody:    string(raw),
	}
}

// ── Assertion helpers ─────────────────────────────────────────────────────────

func assertStatus(t *testing.T, got, want int, r response) {
	t.Helper()
	if got != want {
		t.Errorf("status: got %d, want %d\nbody: %s", got, want, r.RawBody)
	}
}

func assertKey(t *testing.T, r response, key string) any {
	t.Helper()
	v, ok := r.Body[key]
	if !ok {
		t.Errorf("expected key %q in response, got: %s", key, r.RawBody)
	}
	return v
}

func assertStringField(t *testing.T, r response, key, want string) {
	t.Helper()
	v := assertKey(t, r, key)
	if v == nil {
		return
	}
	got, ok := v.(string)
	if !ok {
		t.Errorf("field %q: expected string, got %T", key, v)
		return
	}
	if want != "" && got != want {
		t.Errorf("field %q: got %q, want %q", key, got, want)
	}
}

func assertNonEmptyString(t *testing.T, r response, key string) string {
	t.Helper()
	v := assertKey(t, r, key)
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok || s == "" {
		t.Errorf("field %q: expected non-empty string, got %v", key, v)
	}
	return s
}

func assertNestedStringField(t *testing.T, r response, outerKey, innerKey, want string) {
	t.Helper()
	outer := assertKey(t, r, outerKey)
	if outer == nil {
		return
	}
	nested, ok := outer.(map[string]any)
	if !ok {
		t.Errorf("field %q: expected object, got %T", outerKey, outer)
		return
	}
	got, ok := nested[innerKey].(string)
	if !ok {
		t.Errorf("field %q.%q: expected string", outerKey, innerKey)
		return
	}
	if want != "" && got != want {
		t.Errorf("field %q.%q: got %q, want %q", outerKey, innerKey, got, want)
	}
}

// extractString safely pulls a string from a parsed body
func extractString(r response, key string) string {
	if v, ok := r.Body[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// extractNestedString pulls a string from a nested object in the body
func extractNestedString(r response, outerKey, innerKey string) string {
	if outer, ok := r.Body[outerKey]; ok {
		if m, ok := outer.(map[string]any); ok {
			if s, ok := m[innerKey].(string); ok {
				return s
			}
		}
	}
	return ""
}
