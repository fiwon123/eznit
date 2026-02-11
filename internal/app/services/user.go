package services

import (
	"fmt"

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

func (services *ServicesData) CreateUser(req dto.UserCreate) (dto.UserResponse, bool) {

	db := services.db

	if db.UserExists(req.Email) {
		fmt.Println("user already exists")
		return dto.UserResponse{}, false
	}

	if !db.CreateUser(model.User{
		Email:    req.Email,
		Password: req.Password,
	}) {
		return dto.UserResponse{}, false
	}

	return dto.UserResponse{
		Email:    req.Email,
		Password: req.Password,
	}, true
}

func (services *ServicesData) DeleteUser(req dto.UserDelete) (dto.UserResponse, bool) {

	db := services.db

	user := db.GetUser(req.Id)
	if user == nil {
		return dto.UserResponse{}, false
	}

	if !db.UserExists(user.Email) {
		return dto.UserResponse{}, false
	}

	if !db.DeleteUser(*user) {
		return dto.UserResponse{}, false
	}

	return dto.UserResponse{
		Email:    user.Email,
		Password: user.Password,
	}, true
}

func (services *ServicesData) UpdateUser(req dto.UserUpdate) (dto.UserResponse, bool) {

	db := services.db

	user := db.GetUser(req.Id)
	if user == nil {
		return dto.UserResponse{}, false
	}

	user.Email = req.Email
	user.Password = req.Password

	if !db.UpdateUser(*user) {
		return dto.UserResponse{}, false
	}

	return dto.UserResponse{
		Email:    user.Email,
		Password: user.Password,
	}, true
}
