package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func startTestServer(t *testing.T, envOverrides ...string) (baseURL string, cancel context.CancelFunc) {
	t.Helper()

	port := freePort(t)
	t.Setenv("ARI_PORT", fmt.Sprintf("%d", port))
	t.Setenv("ARI_HOST", "127.0.0.1")
	t.Setenv("ARI_DATA_DIR", t.TempDir())
	t.Setenv("ARI_DATABASE_URL", "")                              // use embedded PG
	t.Setenv("ARI_EMBEDDED_PG_PORT", fmt.Sprintf("%d", freePort(t))) // avoid port conflicts

	// Apply optional env overrides (key=value pairs)
	for _, kv := range envOverrides {
		parts := [2]string{}
		if idx := len(kv); idx > 0 {
			for i := 0; i < len(kv); i++ {
				if kv[i] == '=' {
					parts[0] = kv[:i]
					parts[1] = kv[i+1:]
					break
				}
			}
		}
		if parts[0] != "" {
			t.Setenv(parts[0], parts[1])
		}
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, "test-version", 0)
	}()

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	// Register cleanup unconditionally to avoid goroutine leaks
	t.Cleanup(func() {
		cancelFn()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("runServer returned error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("runServer did not return after cancellation")
		}
	})

	// Wait for server to be ready.
	// REQ-NFR-002 requires production startup under 10s (warm PG cache).
	// Tests use per-instance RuntimePath isolation, so cold binary extraction
	// can take 30-60s. Use a generous timeout here; production startup time
	// is validated separately in Task 10.2.
	start := time.Now()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Logf("server became healthy in %s", time.Since(start))
				return baseURL, cancelFn
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("server did not become healthy within timeout (waited %s)", time.Since(start))
	return "", nil
}

func TestIntegration_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test (requires embedded PG)")
	}

	baseURL, cancel := startTestServer(t)

	// Verify health endpoint
	resp, err := http.Get(baseURL + "/api/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var health map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if health["status"] != "ok" {
		t.Errorf("health status = %q, want %q", health["status"], "ok")
	}
	if health["version"] != "test-version" {
		t.Errorf("health version = %q, want %q", health["version"], "test-version")
	}

	// Cancel and verify clean shutdown
	cancel()
}

func TestIntegration_HealthEndpoint_FullStack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t)

	resp, err := http.Get(baseURL + "/api/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var health map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if health["status"] != "ok" {
		t.Errorf("health status = %q, want %q", health["status"], "ok")
	}
}

func TestIntegration_UnknownRoute_Returns404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t)

	resp, err := http.Get(baseURL + "/api/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var got map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["error"] != "Not found" {
		t.Errorf("error = %q, want %q", got["error"], "Not found")
	}
	if got["code"] != "NOT_FOUND" {
		t.Errorf("code = %q, want %q", got["code"], "NOT_FOUND")
	}
}
