package handlers

import (
	"encoding/json"
	"net/http"

	"apigate-proxy/models"
	"apigate-proxy/service"
)

type LoggerHandler struct {
	Service *service.LoggerService
}

func NewLoggerHandler(svc *service.LoggerService) *LoggerHandler {
	return &LoggerHandler{Service: svc}
}

func (h *LoggerHandler) LogRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.LogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Capture User-Agent from header if not in body
	if req.UserAgent == "" {
		req.UserAgent = r.UserAgent()
	}

	// Basic Validation (from prompt)
	if req.IPAddress == "" || req.Email == "" || req.UserAgent == "" || req.HTTPMethod == "" || req.Endpoint == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.AllowResponse{ // Reusing generic response structure or custom?
			Allow:  false, // Not applicable really
			Status: "failure",
			Error:  "Missing required fields",
		})
		return
	}

	// Defaults (from prompt)
	if req.EventType == "" {
		req.EventType = req.Endpoint
	}

	// Queue the log
	h.Service.QueueLog(req)

	// Return success immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// "communicate to the client using the same api structure"
	// Prompt didn't specify the exact success response structure for /api/log,
	// but the examples imply we just accept it.
	// I'll return a simple success JSON.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Log queued",
	})
}
