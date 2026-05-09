package create

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
)

func TestWaitForSimulationState_Success(t *testing.T) {
	origPollInterval := waitForSimulationStatePollInterval
	origMaxWaitTime := waitForSimulationStateMaxWaitTime
	waitForSimulationStatePollInterval = 10 * time.Millisecond
	waitForSimulationStateMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForSimulationStatePollInterval = origPollInterval
		waitForSimulationStateMaxWaitTime = origMaxWaitTime
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/sim-123/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		state := "REQUESTING"
		if atomic.AddInt32(&calls, 1) >= 2 {
			state = "ACTIVE"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      "sim-123",
			"name":    "demo",
			"state":   state,
			"created": "2026-05-09T00:00:00Z",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	if err := cmd.WaitForSimulationState(client, "sim-123", "ACTIVE"); err != nil {
		t.Fatalf("WaitForSimulationState failed: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForSimulationState_ReachesInactive(t *testing.T) {
	origPollInterval := waitForSimulationStatePollInterval
	origMaxWaitTime := waitForSimulationStateMaxWaitTime
	waitForSimulationStatePollInterval = 10 * time.Millisecond
	waitForSimulationStateMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForSimulationStatePollInterval = origPollInterval
		waitForSimulationStateMaxWaitTime = origMaxWaitTime
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/sim-123/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		state := "IMPORTING"
		if atomic.AddInt32(&calls, 1) >= 2 {
			state = "INACTIVE"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      "sim-123",
			"name":    "demo",
			"state":   state,
			"created": "2026-05-09T00:00:00Z",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	if err := cmd.WaitForSimulationState(client, "sim-123", "INACTIVE"); err != nil {
		t.Fatalf("WaitForSimulationState failed: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 polls, got %d", got)
	}
}

func TestWaitForSimulationState_InvalidStateFails(t *testing.T) {
	origPollInterval := waitForSimulationStatePollInterval
	origMaxWaitTime := waitForSimulationStateMaxWaitTime
	waitForSimulationStatePollInterval = 10 * time.Millisecond
	waitForSimulationStateMaxWaitTime = 200 * time.Millisecond
	defer func() {
		waitForSimulationStatePollInterval = origPollInterval
		waitForSimulationStateMaxWaitTime = origMaxWaitTime
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/simulations/sim-123/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      "sim-123",
			"name":    "demo",
			"state":   "INVALID",
			"created": "2026-05-09T00:00:00Z",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-token")
	cmd := &Command{}

	err := cmd.WaitForSimulationState(client, "sim-123", "ACTIVE")
	if err == nil {
		t.Fatal("expected WaitForSimulationState to fail for INVALID state")
	}
	if !strings.Contains(err.Error(), "INVALID") {
		t.Fatalf("expected INVALID state error, got %v", err)
	}
}
