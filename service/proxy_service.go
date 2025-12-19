package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"apigate-proxy/config"
	"apigate-proxy/models"
	"apigate-proxy/utils"
)

type ProxyService struct {
	config *config.Config
	client *http.Client

	mu sync.RWMutex
	// Cache for current window
	currentCache map[string]bool
	// Cache being built for next window
	pendingCache map[string]bool
	// Keys collected for the next batch
	batchedKeys map[string]struct{}
	// Warmup flag
	warmUp bool

	// Metrics
	totalReqs       int64
	individualCalls int64
	lastBatchSize   int64
}

func NewProxyService(cfg *config.Config) *ProxyService {
	return &ProxyService{
		config:       cfg,
		client:       &http.Client{Timeout: 10 * time.Second},
		currentCache: make(map[string]bool),
		pendingCache: nil,
		batchedKeys:  make(map[string]struct{}),
		warmUp:       true,
	}
}

func (s *ProxyService) Start() {
	winSec := s.config.WindowSeconds
	if winSec < 5 {
		winSec = 20
	}
	windowDuration := time.Duration(winSec) * time.Second
	// Calculate durations
	fetchOffset := 5 * time.Second
	fetchDuration := windowDuration - fetchOffset
	if fetchDuration <= 0 {
		fetchDuration = 1 * time.Second
	}

	go func() {
		log.Printf("[ProxyService] Starting background worker. Window: %v, FetchOffset: %v", windowDuration, fetchOffset)

		start := time.Now()
		nextFetch := start.Add(fetchDuration)
		nextSwap := start.Add(windowDuration)

		for {
			now := time.Now()

			// 1. Wait for prefetch time
			if wait := nextFetch.Sub(now); wait > 0 {
				time.Sleep(wait)
			}
			s.prefetch()
			nextFetch = nextFetch.Add(windowDuration)

			// 2. Wait for window swap time
			now = time.Now()
			if wait := nextSwap.Sub(now); wait > 0 {
				time.Sleep(wait)
			}
			s.swapCache()
			nextSwap = nextSwap.Add(windowDuration)
		}
	}()
}

// EncryptEmail encrypts the email if encryption is enabled and key is configured.
func (s *ProxyService) EncryptEmail(email string) string {
	if email == "" || !s.config.EmailEncryptionEnabled || s.config.EmailEncryptionKey == "" {
		return email
	}
	if s.config.EmailEncryptionFormat == "numeric" {
		return utils.OneWayKeyedHashNumeric([]byte(s.config.EmailEncryptionKey), email)
	}
	return utils.OneWayKeyedHash([]byte(s.config.EmailEncryptionKey), email)
}

func (s *ProxyService) Check(req models.AllowRequest) (models.AllowResponse, error) {
	atomic.AddInt64(&s.totalReqs, 1)

	// 1. Encrypt email (if configured) and track keys for next window
	reqFor := req // copy
	if req.Email != "" {
		reqFor.Email = s.EncryptEmail(req.Email)
	}
	s.trackKeys(reqFor)

	s.mu.RLock()
	warmUp := s.warmUp
	s.mu.RUnlock()

	// 2. Warmup Phase
	if warmUp {
		return models.AllowResponse{Allow: true, Status: "success", Message: "Warmup: Allowed"}, nil
	}

	// 3. Check Cache
	s.mu.RLock()
	decision, found := s.getFromCache(reqFor)
	s.mu.RUnlock()

	if found {
		msg := "Cache Hit"
		if !decision {
			msg = "Cache Hit: Blocked"
		}
		return models.AllowResponse{Allow: decision, Status: "success", Message: msg}, nil
	}

	// 4. Cache Miss -> Fallback to Batch Upstream
	// We use the batch endpoint even for a single request context to get status for each key separately.
	// This allows us to cache both ALLOW and BLOCK statuses for specific keys.

	atomic.AddInt64(&s.individualCalls, 1)

	// Collect keys from this request
	keys := make([]string, 0, 3)
	if reqFor.IPAddress != "" {
		keys = append(keys, reqFor.IPAddress)
	}
	if reqFor.Email != "" {
		// reqFor.Email is a one-way hash when key configured
		keys = append(keys, reqFor.Email)
	}
	if reqFor.UserAgent != "" {
		keys = append(keys, utils.CompressUserAgent(reqFor.UserAgent))
	}

	if len(keys) == 0 {
		return models.AllowResponse{Allow: false, Status: "error", Message: "No keys provided"}, nil
	}

	// Call Upstream Batch
	results, err := s.callUpstreamBatch(keys)
	if err != nil {
		return models.AllowResponse{}, err
	}

	// Process Results & Update Cache
	s.mu.Lock()
	allowed := true
	for _, item := range results {
		// Update cache for this specific key
		s.currentCache[item.Key] = item.Allow
		// If any part of the request is blocked, the whole request is blocked
		if !item.Allow {
			allowed = false
		}
	}
	s.mu.Unlock()

	msg := "Allowed (Live Check)"
	if !allowed {
		msg = "Blocked (Live Check)"
	}

	return models.AllowResponse{Allow: allowed, Status: "success", Message: msg}, nil
}

func (s *ProxyService) trackKeys(req models.AllowRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.IPAddress != "" {
		s.batchedKeys[req.IPAddress] = struct{}{}
	}
	if req.Email != "" {
		s.batchedKeys[req.Email] = struct{}{}
	}
	if req.UserAgent != "" {
		// Hash the UA before tracking
		hashedUA := utils.CompressUserAgent(req.UserAgent)
		s.batchedKeys[hashedUA] = struct{}{}
	}
}

func (s *ProxyService) getFromCache(req models.AllowRequest) (bool, bool) {
	// Default to true (allow) only if ALL keys are present and true.
	// If ANY key is present and false (block), then BLOCK.
	// If keys are missing, then return found=false (Cache Miss).

	ipStatus, ipKnown := s.currentCache[req.IPAddress]
	emailStatus, emailKnown := s.currentCache[req.Email]

	// Logic:
	// If IP is known and blocked -> Block
	if req.IPAddress != "" && ipKnown && !ipStatus {
		return false, true
	}
	// If Email is known and blocked -> Block
	if req.Email != "" && emailKnown && !emailStatus {
		return false, true
	}

	// Check UA
	var uaStatus, uaKnown bool
	if req.UserAgent != "" {
		hashedUA := utils.CompressUserAgent(req.UserAgent)
		uaStatus, uaKnown = s.currentCache[hashedUA]
		if uaKnown && !uaStatus {
			return false, true
		}
	}

	// If both are required and known and allowed -> Allow
	// What if only one is provided?
	ipOk := (req.IPAddress == "") || (ipKnown && ipStatus)
	emailOk := (req.Email == "") || (emailKnown && emailStatus)
	uaOk := (req.UserAgent == "") || (uaKnown && uaStatus)

	if ipOk && emailOk && uaOk {
		// Both are "OK" (either empty or known-allow).
		// But we must ensure at least one was actually checked?
		// If input is empty, that's an error elsewhere, but here:
		if req.IPAddress == "" && req.Email == "" && req.UserAgent == "" {
			return false, false // Nothing to check
		}

		// If we have a partial miss (e.g. IP known allow, Email unknown), we treat as MISS.
		if (req.IPAddress != "" && !ipKnown) || (req.Email != "" && !emailKnown) || (req.UserAgent != "" && !uaKnown) {
			return false, false
		}

		return true, true
	}

	// Fallback (should be covered by miss logic)
	return false, false
}

func (s *ProxyService) prefetch() {
	s.mu.Lock()
	// Collect keys to fetch
	keys := make([]string, 0, len(s.batchedKeys))
	for k := range s.batchedKeys {
		keys = append(keys, k)
	}

	// Reset collected keys for the next window tracking.
	// We reset here so that any new requests coming in during the 'fetch gap'
	// start populating the batch for the subsequent window.
	s.batchedKeys = make(map[string]struct{})
	s.mu.Unlock()

	if len(keys) == 0 {
		return
	}

	// Call Upstream
	// Note: Doing this outside lock
	atomic.StoreInt64(&s.lastBatchSize, int64(len(keys)))
	go func(batchKeys []string) {
		log.Printf("Prefetching %d keys for next window...", len(batchKeys))
		results, err := s.callUpstreamBatch(batchKeys)
		if err != nil {
			log.Printf("[ProxyService] Error prefetching batch: %v", err)
			return
		}

		newCache := make(map[string]bool)
		for _, cx := range results {
			newCache[cx.Key] = cx.Allow
		}

		s.mu.Lock()
		s.pendingCache = newCache
		s.mu.Unlock()
		log.Println("Prefetch complete. Pending cache updated.")
	}(keys)
}

func (s *ProxyService) swapCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.warmUp = false

	// Swap the cache
	if s.pendingCache != nil {
		s.currentCache = s.pendingCache
		s.pendingCache = nil
	} else {
		// If fetch failed or no keys were pending, ensure we have a valid empty cache
		s.currentCache = make(map[string]bool)
	}

	// Logging Efficiency Stats
	total := atomic.SwapInt64(&s.totalReqs, 0)
	individual := atomic.SwapInt64(&s.individualCalls, 0)
	batchSize := atomic.SwapInt64(&s.lastBatchSize, 0)

	log.Printf("[Window Stats] Total Requests: %d, Individual Upstream Calls: %d, Batch Keys Prefetched: %d",
		total, individual, batchSize)
}

// Http Utils

func (s *ProxyService) callUpstreamBatch(keys []string) ([]models.BatchAllowResponseItem, error) {
	url := fmt.Sprintf("%s/api/allow/batch", s.config.UpstreamBaseURL)
	body, _ := json.Marshal(keys)

	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if s.config.UpstreamAPIKey != "" {
		r.Header.Set("X-API-Key", s.config.UpstreamAPIKey)
	}

	resp, err := s.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status: %d", resp.StatusCode)
	}

	var result []models.BatchAllowResponseItem
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}
