package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"apigate-proxy/config"
	"apigate-proxy/handlers"
	"apigate-proxy/service"
)

func main() {
	// Load Configuration
	cfg := config.LoadConfig()

	// Initialize Service
	svc := service.NewProxyService(cfg)
	svc.Start()

	// Initialize Handlers
	proxyHandler := handlers.NewProxyHandler(svc)

	loggerSvc := service.NewLoggerService(cfg)
	loggerSvc.Start()
	loggerHandler := handlers.NewLoggerHandler(loggerSvc)

	// Router
	r := mux.NewRouter()
	r.HandleFunc("/api/allow", proxyHandler.AllowDecisionHandler).Methods("POST")
	r.HandleFunc("/api/encrypt-email", proxyHandler.EncryptEmailHandler).Methods("GET")
	r.HandleFunc("/api/log", loggerHandler.LogRequestHandler).Methods("POST")

	// Start Server

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: r,
	}

	go func() {
		log.Printf("Proxy Server starting on port %s", cfg.ServerPort)
		log.Printf("Upstream Configured: %s", cfg.UpstreamBaseURL)
		log.Printf("Window Size: %ds", cfg.WindowSeconds)
		log.Printf("Log Flush: %ds, Batch Size: %d", cfg.LogFlushInterval, cfg.LogBatchSize)
		if cfg.UpstreamAPIKey != "" {
			log.Printf("Upstream API Key: Configured (Length: %d)", len(cfg.UpstreamAPIKey))
		} else {
			log.Printf("Upstream API Key: NOT Configured")
		}

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Context for server shutdown (give it 5 seconds to finish requests)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	loggerSvc.Stop()
	log.Println("Server exited properly")
}
