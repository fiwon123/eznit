package helper

import (
	"encoding/json"
	"maps"
	"net/http"

	"github.com/fiwon123/eznit/internal/data/types"
)

func WriteJSON(w http.ResponseWriter, status int, data types.Envelope, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	js = append(js, '\n')
	maps.Copy(w.Header(), headers)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)
	return nil
}
