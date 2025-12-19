package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"apigate-proxy/config"
	"apigate-proxy/models"
	"apigate-proxy/utils"
)

func TestProxyService_Flow(t *testing.T) {
	// 1. Mock Upstream Server
	// Use a fixed 32-byte key for tests
	testKey := "0123456789abcdef0123456789abcdef"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/allow" {
			// Individual
			var req models.AllowRequest
			json.NewDecoder(r.Body).Decode(&req)

			// Block 1.2.3.4, Allow others
			if req.IPAddress == "1.2.3.4" {
				json.NewEncoder(w).Encode(models.AllowResponse{Allow: false})
			} else {
				json.NewEncoder(w).Encode(models.AllowResponse{Allow: true})
			}
		} else if r.URL.Path == "/api/allow/batch" {
			// Batch
			var keys []string
			json.NewDecoder(r.Body).Decode(&keys)
			var res []models.BatchAllowResponseItem
			// Precompute hashed blocked email
			blockedHash := utils.OneWayKeyedHash([]byte(testKey), "blocked@test.com")
			for _, k := range keys {
				allow := true
				// If key matches blocked hashed email, deny
				if k == blockedHash {
					allow = false
				} else if k == "1.2.3.4" {
					allow = false
				}
				res = append(res, models.BatchAllowResponseItem{Key: k, Allow: allow})
			}
			json.NewEncoder(w).Encode(res)
		}
	}))
	defer upstream.Close()

	// 2. Setup Config
	// Use the same fixed 32-byte key declared above for tests
	cfg := &config.Config{
		ServerPort:             "9090",
		UpstreamBaseURL:        upstream.URL,
		WindowSeconds:          2, // Short window for testing
		EmailEncryptionKey:     testKey,
		EmailEncryptionEnabled: true,
	}

	svc := NewProxyService(cfg)
	// We do NOT call svc.Start() because we want to manually control prefetch/swap for deterministic testing.
	// But `Start` uses internal goroutine.
	// Let's modify `Start` or just call methods manually.
	// Since `Start` is independent, we can just call `prefetch` and `swapCache` manually in this test.

	// A. Warmup Phase
	req1 := models.AllowRequest{IPAddress: "1.2.3.4"}
	resp1, _ := svc.Check(req1)
	if !resp1.Allow || resp1.Message != "Warmup: Allowed" {
		t.Errorf("Expected Warmup Allowed, got %v", resp1)
	}

	// Track some keys
	svc.Check(models.AllowRequest{IPAddress: "5.6.7.8"}) // Safe IP
	svc.Check(models.AllowRequest{Email: "blocked@test.com"})

	// Verify tracked keys
	svc.mu.RLock()
	if _, ok := svc.batchedKeys["1.2.3.4"]; !ok {
		t.Error("1.2.3.4 not tracked")
	}
	svc.mu.RUnlock()

	// B. Trigger Prefetch (Simulate T-5s)
	svc.prefetch()
	// Wait for goroutine
	time.Sleep(100 * time.Millisecond)

	svc.mu.RLock()
	if svc.pendingCache == nil {
		t.Error("Pending cache not built")
	}
	// Check content of pending cache
	if allow, ok := svc.pendingCache["1.2.3.4"]; !ok || allow {
		t.Errorf("1.2.3.4 should be in pending cache and blocked (false), got %v", allow)
	}
	if allow, ok := svc.pendingCache["5.6.7.8"]; !ok || !allow {
		t.Errorf("5.6.7.8 should be in pending cache and allowed (true), got %v", allow)
	}
	svc.mu.RUnlock()

	// C. Trigger Swap (Simulate Window End)
	svc.swapCache()

	svc.mu.RLock()
	if svc.warmUp {
		t.Error("Warmup should be off")
	}
	if len(svc.currentCache) == 0 {
		t.Error("Current cache empty after swap")
	}
	svc.mu.RUnlock()

	// D. Verify Cache Hit (Window 2)
	// 1.2.3.4 is blocked in cache
	resp2, _ := svc.Check(req1)
	if resp2.Allow {
		t.Error("Expected 1.2.3.4 to be blocked from cache")
	}
	if resp2.Message != "Cache Hit: Blocked" {
		t.Errorf("Expected Cache Hit message, got %s", resp2.Message)
	}

	// 5.6.7.8 is allowed in cache
	resp3, _ := svc.Check(models.AllowRequest{IPAddress: "5.6.7.8"})
	if !resp3.Allow {
		t.Error("Expected 5.6.7.8 to be allowed from cache")
	}

	// E. Unknown Key (Cache Miss -> Individual)
	// 9.9.9.9 is new. Should be miss -> upstream (Allow).
	resp4, _ := svc.Check(models.AllowRequest{IPAddress: "9.9.9.9"})
	if !resp4.Allow {
		t.Error("Expected 9.9.9.9 to be allowed (upstream)")
	}

	// F. Verify Individual Caching Optimization
	// Since 9.9.9.9 was allowed, it should be added to currentCache immediately.
	svc.mu.RLock()
	cached, ok := svc.currentCache["9.9.9.9"]
	svc.mu.RUnlock()

	if !ok || !cached {
		t.Error("Optimization failed: 9.9.9.9 should be added to currentCache after individual block check success")
	}

	// Verify subsequent hit
	resp5, _ := svc.Check(models.AllowRequest{IPAddress: "9.9.9.9"})
	if resp5.Message != "Cache Hit" {
		t.Errorf("Expected immediate Cache Hit for 9.9.9.9, got %s", resp5.Message)
	}
}
