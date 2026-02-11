package dto

type UserCreate struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserUpdate struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserDelete struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
