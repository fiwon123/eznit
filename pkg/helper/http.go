package helper

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/pkg/types"
)

func SendDataJson[T any](w http.ResponseWriter, statusCode int, data T) {

	env := types.Envelope[T]{
		StatusCode: statusCode,
		Data:       data,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(env)
}

func SendMessageJson(w http.ResponseWriter, statusCode int, msg string) {

	env := types.Envelope[any]{
		StatusCode: statusCode,
		Msg:        msg,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(env)
}

func SendErrorJson(w http.ResponseWriter, statusCode int, msg string) {

	env := types.Envelope[any]{
		StatusCode: statusCode,
		Error:      msg,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(env)
}
