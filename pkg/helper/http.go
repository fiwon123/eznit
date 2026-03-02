package helper

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/pkg/types"
)

func SendDataJson(w http.ResponseWriter, statusCode int, data interface{}) {

	env := types.Envelope{
		StatusCode: statusCode,
		Data:       data,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(env)
}

func SendMessageJson(w http.ResponseWriter, statusCode int, msg string) {

	env := types.Envelope{
		StatusCode: statusCode,
		Msg:        msg,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(env)
}

func SendErrorJson(w http.ResponseWriter, statusCode int, msg string) {

	env := types.Envelope{
		StatusCode: statusCode,
		Error:      msg,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(env)
}
