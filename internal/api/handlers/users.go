package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fiwon123/eznit/internal/app/dto"
)

func (handlers *handlersData) getUsersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		users := handlers.services.GetUsers()

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "Application/json")
		json.NewEncoder(w).Encode(users)
	}
}

func (handlers *handlersData) getUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		user, found := handlers.services.GetUser(id)
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
}

func (handlers *handlersData) createUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user dto.UserCreate

		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			http.Error(w, "Failed to decode request", http.StatusBadRequest)
			return
		}

		resp, ok := handlers.services.CreateUser(user)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

func (handlers *handlersData) deleteUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		_, exists := users[id]
		if !exists {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		delete(users, id)

		response := map[string]string{
			"message": fmt.Sprintf("User %s is deleted successfully", id),
		}
		json.NewEncoder(w).Encode(response)
	}
}

func (handlers *handlersData) updateUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var update dto.UserUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		update.ID = id
		users[id] = update

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(update)
	}
}
