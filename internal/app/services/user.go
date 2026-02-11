package services

import (
	"github.com/fiwon123/eznit/internal/app/dto"
)

func (services *ServicesData) GetUsers() []dto.UserResponse {

	usersModel := services.db.GetUsers()

	resp := []dto.UserResponse{}
	for _, user := range usersModel {
		resp = append(resp, dto.UserResponse{
			Email:    user.Email,
			Password: user.Password,
		})
	}

	return resp
}

func CreateUser() {

}
