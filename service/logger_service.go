package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"apigate-proxy/config"
	"apigate-proxy/models"
	"apigate-proxy/utils"
)

type LoggerService struct {
	config *config.Config
	client *http.Client

	mu        sync.Mutex
	buffer    []models.LogRequest
	flushChan chan []models.LogRequest // To handle flush trigger
}

func NewLoggerService(cfg *config.Config) *LoggerService {
	return &LoggerService{
		config:    cfg,
		client:    &http.Client{Timeout: 10 * time.Second},
		buffer:    make([]models.LogRequest, 0, cfg.LogBatchSize),
		flushChan: make(chan []models.LogRequest, 10), // Buffered chan
	}
}

func (s *LoggerService) Start() {

	// Start ticker
	go func() {
		interval := time.Duration(s.config.LogFlushInterval) * time.Second
		if interval < 1*time.Second {
			interval = 10 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			s.triggerFlush()
		}
	}()
}

func (s *LoggerService) QueueLog(req models.LogRequest) {
	// Encrypt email immediately if configured
	if s.config.EmailEncryptionKey != "" && req.Email != "" {
		if s.config.EmailEncryptionFormat == "numeric" {
			req.Email = utils.OneWayKeyedHashNumeric([]byte(s.config.EmailEncryptionKey), req.Email)
		} else {
			req.Email = utils.OneWayKeyedHash([]byte(s.config.EmailEncryptionKey), req.Email)
		}
	}

	s.mu.Lock()
	s.buffer = append(s.buffer, req)
	shouldFlush := len(s.buffer) >= s.config.LogBatchSize
	s.mu.Unlock()

	// If batch size reached, trigger flush immediately (async)
	if shouldFlush {
		s.triggerFlush()
	}
}

// triggerFlush sends the current buffer to the upstream logging endpoint.
// It resets the buffer and spawns a goroutine to handle the network call.
func (s *LoggerService) triggerFlush() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}

	// Create a copy to flush
	batch := make([]models.LogRequest, len(s.buffer))
	copy(batch, s.buffer)

	// Reset buffer
	s.buffer = s.buffer[:0]
	s.mu.Unlock()

	// Send to worker
	// Send to worker asynchronously
	go s.sendBatch(batch)
}

func (s *LoggerService) sendBatch(batch []models.LogRequest) {
	if len(batch) == 0 {
		return
	}

	url := fmt.Sprintf("%s/api/logs", s.config.UpstreamBaseURL)
	// Emails are already encrypted in QueueLog

	body, _ := json.Marshal(batch)

	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("[Logger] Error creating request: %v", err)
		return
	}
	r.Header.Set("Content-Type", "application/json")
	if s.config.UpstreamAPIKey != "" {
		r.Header.Set("X-API-Key", s.config.UpstreamAPIKey)
	}

	resp, err := s.client.Do(r)
	if err != nil {
		log.Printf("[Logger] Error sending batch logs: %v", err)
		// Retry logic could go here (e.g. put back in buffer), but simpler to drop/log for now.
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("[Logger] Upstream returned error: %d", resp.StatusCode)
	} else {
		log.Printf("[Logger] Flushed batch of %d data points to server.", len(batch))
	}
}

// Stop flushes any remaining logs synchronously before shutdown
func (s *LoggerService) Stop() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}
	batch := make([]models.LogRequest, len(s.buffer))
	copy(batch, s.buffer)
	s.buffer = s.buffer[:0]
	s.mu.Unlock()

	log.Println("[LoggerService] Flushing remaining logs on shutdown...")
	s.sendBatch(batch)
}
