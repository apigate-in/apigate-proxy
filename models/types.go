package models

// AllowRequest represents the body of the individual check request.
type AllowRequest struct {
	IPAddress string `json:"ip_address"`
	Email     string `json:"email"`      // Can be Email OR any unique User ID
	UserAgent string `json:"user_agent"` // Optional, can be populated from header
}

// AllowResponse represents the response from the individual check.
type AllowResponse struct {
	Allow         bool     `json:"allow"`
	Status        string   `json:"status"`
	Message       string   `json:"message,omitempty"`
	Error         string   `json:"error,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
}

// BatchAllowResponseItem represents a single item in the batch response.
type BatchAllowResponseItem struct {
	Key   string `json:"key"`
	Type  string `json:"type"` // "ip" or "email" or "user_agent"
	Allow bool   `json:"allow"`
}

// BatchAllowRequest represents the body for the upstream batch request.
// It is just an array of strings: string[]
type BatchAllowRequest []string

// LogRequest represents the full request details for logging.
type LogRequest struct {
	IPAddress    string `json:"ip_address"`
	Email        string `json:"email"`
	UserAgent    string `json:"user_agent"`
	HTTPMethod   string `json:"http_method"`
	Endpoint     string `json:"endpoint"`
	EventType    string `json:"event_type,omitempty"`
	Username     string `json:"username,omitempty"`
	ResponseCode int    `json:"response_code,omitempty"`
	TrackRequest bool   `json:"track_request"`
}

// LogResponse represents the response to the client for the log endpoint.
type LogResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
