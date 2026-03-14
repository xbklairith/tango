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

func startTestServer(t *testing.T) (baseURL string, cancel context.CancelFunc) {
	t.Helper()

	port := freePort(t)
	t.Setenv("ARI_PORT", fmt.Sprintf("%d", port))
	t.Setenv("ARI_HOST", "127.0.0.1")
	t.Setenv("ARI_DATA_DIR", t.TempDir())
	t.Setenv("ARI_DATABASE_URL", "") // use embedded PG

	ctx, cancelFn := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, "test-version")
	}()

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server to be ready (poll health endpoint)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
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
				return baseURL, cancelFn
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	cancelFn()
	t.Fatal("server did not become healthy within timeout")
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
