package types

type Envelope struct {
	StatusCode int         `json:"status_code,omitempty"`
	Data       interface{} `json:"datas,omitempty"`
	Msg        string      `json:"message,omitempty"`
	Error      string      `json:"error,omitempty"`
	Timestamp  string      `json:"timestamp,omitempty"`
}
