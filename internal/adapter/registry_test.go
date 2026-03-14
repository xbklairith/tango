package adapter_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/xb/ari/internal/adapter"
)

// stubAdapter implements adapter.Adapter for testing.
type stubAdapter struct {
	adapterType string
}

func (s *stubAdapter) Type() string { return s.adapterType }
func (s *stubAdapter) Execute(_ context.Context, _ adapter.InvokeInput, _ adapter.Hooks) (adapter.InvokeResult, error) {
	return adapter.InvokeResult{}, nil
}
func (s *stubAdapter) TestEnvironment(_ adapter.TestLevel) (adapter.TestResult, error) {
	return adapter.TestResult{Available: true}, nil
}
func (s *stubAdapter) Models() []adapter.ModelDefinition { return nil }

func TestRegistry_RegisterAndResolve(t *testing.T) {
	reg := adapter.NewRegistry()
	stub := &stubAdapter{adapterType: "test"}
	reg.Register(stub)

	got, err := reg.Resolve("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type() != "test" {
		t.Fatalf("expected type %q, got %q", "test", got.Type())
	}
}

func TestRegistry_ResolveUnknown(t *testing.T) {
	reg := adapter.NewRegistry()
	_, err := reg.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown adapter type")
	}
	if !strings.Contains(err.Error(), "no adapter registered") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRegistry_MarkUnavailable(t *testing.T) {
	reg := adapter.NewRegistry()
	stub := &stubAdapter{adapterType: "broken"}
	reg.Register(stub)
	reg.MarkUnavailable("broken", "missing binary")

	_, err := reg.Resolve("broken")
	if err == nil {
		t.Fatal("expected error for unavailable adapter")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRegistry_ConcurrentReads(t *testing.T) {
	reg := adapter.NewRegistry()
	stub := &stubAdapter{adapterType: "concurrent"}
	reg.Register(stub)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := reg.Resolve("concurrent")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got == nil {
				t.Error("got nil adapter")
			}
		}()
	}
	wg.Wait()
}
