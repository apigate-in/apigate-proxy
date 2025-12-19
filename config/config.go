package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort            string
	UpstreamBaseURL       string
	WindowSeconds         int
	LogFlushInterval      int // Seconds
	LogBatchSize          int
	UpstreamAPIKey        string
	EmailEncryptionKey    string
	EmailEncryptionFormat string
}

func LoadConfig() *Config {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using defaults/environment variables")
	}

	// Defaults
	port := "8080"
	upstreamURL := "http://localhost:8000" // Default upstream as per prompt
	windowSecs := 20
	logFlush := 10 // Default flush every 10s
	logBatch := 50 // Default batch size 50
	apiKey := ""

	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	if u := os.Getenv("UPSTREAM_BASE_URL"); u != "" {
		upstreamURL = u
	}
	if w := os.Getenv("WINDOW_SECONDS"); w != "" {
		if val, err := strconv.Atoi(w); err == nil {
			windowSecs = val
		}
	}
	if l := os.Getenv("LOG_FLUSH_INTERVAL"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			logFlush = val
		}
	}
	if b := os.Getenv("LOG_BATCH_SIZE"); b != "" {
		if val, err := strconv.Atoi(b); err == nil {
			logBatch = val
		}
	}
	if k := os.Getenv("UPSTREAM_API_KEY"); k != "" {
		apiKey = k
	}
	if e := os.Getenv("EMAIL_ENCRYPTION_KEY"); e != "" {
		// Use as-is
		// It's fine to store raw string here.
		// Load even if empty to allow opt-out when not configured.
		_ = e
	}

	return &Config{
		ServerPort:         port,
		UpstreamBaseURL:    upstreamURL,
		WindowSeconds:      windowSecs,
		LogFlushInterval:   logFlush,
		LogBatchSize:       logBatch,
		UpstreamAPIKey:     apiKey,
		EmailEncryptionKey: os.Getenv("EMAIL_ENCRYPTION_KEY"),
		EmailEncryptionFormat: func() string {
			if f := os.Getenv("EMAIL_ENCRYPTION_FORMAT"); f != "" {
				return f
			}
			return "hex"
		}(),
	}
}
