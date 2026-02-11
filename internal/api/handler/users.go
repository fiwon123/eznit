package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fiwon123/eznit/internal/app/dto"
)

func (config *Config) getUsersHandler(w http.ResponseWriter, r *http.Request) {
	users := config.service.GetUsers()

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "Application/json")
	json.NewEncoder(w).Encode(users)
}

func (config *Config) getUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	user, found := config.service.GetUser(id)
	if found {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(user)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		response := map[string]string{
			"message": fmt.Sprintf("User with id %s not found", id),
		}
		json.NewEncoder(w).Encode(response)
	}
}

func (config *Config) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var user dto.UserCreate

	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	resp, ok := config.service.CreateUser(user)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (config *Config) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	_, ok := config.service.DeleteUser(dto.UserDelete{
		Id: id,
	})
	if !ok {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	response := map[string]string{
		"message": fmt.Sprintf("User %s is deleted successfully", id),
	}

	json.NewEncoder(w).Encode(response)
}

func (config *Config) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req dto.UserUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Id = id
	resp, ok := config.service.UpdateUser(req)
	if !ok {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

}
