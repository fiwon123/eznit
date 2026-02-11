package services

import (
	"github.com/fiwon123/eznit/internal/app/dto"
	"github.com/fiwon123/eznit/internal/domain/model"
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

func (services *ServicesData) GetUser(id string) (dto.UserResponse, bool) {

	user := services.db.GetUser(id)
	if user == nil {
		return dto.UserResponse{}, false
	}

	resp := dto.UserResponse{
		Email:    user.Email,
		Password: user.Password,
	}

	return resp, true
}

func (services *ServicesData) CreateUser(user dto.UserCreate) (dto.UserResponse, bool) {

	db := services.db

	if db.UserExists(user.Email) {
		return dto.UserResponse{}, false
	}

	if !db.CreateUser(model.User{
		Email:    user.Email,
		Password: user.Password,
	}) {
		return dto.UserResponse{}, false
	}

	return dto.UserResponse{
		Email:    user.Email,
		Password: user.Password,
	}, true
}
