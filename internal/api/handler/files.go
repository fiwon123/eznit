package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (config *Config) getFilesHandler(w http.ResponseWriter, r *http.Request) {
	resp, ok := config.service.GetFiles()
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (config *Config) uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Prevents attackers from sending infinite data to crash your server.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	// Parse the multipart form. 8MB stays in RAM, the rest goes to temp files.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "File too big", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Could not find file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	resp, ok := config.service.StorageFile(file, header)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, resp.Msg, header.Filename)
}
