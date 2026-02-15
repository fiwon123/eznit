package users

type CreateRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UpdateRequest struct {
	Id       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type DeleteRequest struct {
	Id string `json:"id"`
}

type MsgResponse struct {
	Msg string `json:"message"`
}

type DataResponse struct {
	Email string `json:"email"`
}
