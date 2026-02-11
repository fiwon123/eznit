package dto

type UserCreate struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserUpdate struct {
	Id       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserDelete struct {
	Id string `json:"id"`
}

type UserResponse struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
