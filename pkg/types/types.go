package types

// A wrapper to send a response for user request
type Envelope[T any] struct {
	StatusCode int    `json:"status_code,omitempty"`
	Data       T      `json:"data,omitempty"`
	Msg        string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
	Timestamp  string `json:"timestamp,omitempty"`
}
