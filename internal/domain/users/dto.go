package users

import "github.com/google/uuid"

// User Data
type UserData struct {
	Email string `json:"email"`
}

// Signup Request
type SignupRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// Login Request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login Response
type LoginResponse struct {
	Token string `json:"token"`
}

// Update Request
type UpdateRequest struct {
	Id       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Password string    `json:"password"`
}

// Delete Request
type DeleteRequest struct {
	Id uuid.UUID `json:"id"`
}
