package handlers

import (
	"encoding/json"
	"net/http"

	"apigate-proxy/models"
	"apigate-proxy/service"
)

type ProxyHandler struct {
	Service *service.ProxyService
}

func NewProxyHandler(svc *service.ProxyService) *ProxyHandler {
	return &ProxyHandler{Service: svc}
}

func (h *ProxyHandler) AllowDecisionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.AllowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Capture User-Agent from header if not in body
	if req.UserAgent == "" {
		req.UserAgent = r.UserAgent()
	}

	// Basic validation
	if req.IPAddress == "" && req.Email == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.AllowResponse{
			Allow:  false,
			Status: "failure",
			Error:  "Missing required fields (ip_address or email/user_id)",
		})
		return
	}

	resp, err := h.Service.Check(req)
	if err != nil {
		// Log error?
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(models.AllowResponse{
			Allow:  false,
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *ProxyHandler) EncryptEmailHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Missing email query parameter", http.StatusBadRequest)
		return
	}

	encrypted := h.Service.EncryptEmail(email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"email":     email,
		"encrypted": encrypted,
	})
}
