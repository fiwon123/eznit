package service

import (
	"fmt"

	"github.com/fiwon123/eznit/internal/app/dto"
	"github.com/fiwon123/eznit/internal/domain/model"
)

func (config *Config) GetUsers() []dto.UserResponse {

	usersModel := config.db.GetUsers()

	resp := []dto.UserResponse{}
	for _, user := range usersModel {
		resp = append(resp, dto.UserResponse{
			Email:    user.Email,
			Password: user.Password,
		})
	}

	return resp
}

func (config *Config) GetUser(id string) (dto.UserResponse, bool) {

	user := config.db.GetUser(id)
	if user == nil {
		return dto.UserResponse{}, false
	}

	resp := dto.UserResponse{
		Email:    user.Email,
		Password: user.Password,
	}

	return resp, true
}

func (config *Config) CreateUser(req dto.UserCreate) (dto.UserResponse, bool) {

	db := config.db

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

func (config *Config) DeleteUser(req dto.UserDelete) (dto.UserResponse, bool) {

	db := config.db

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

func (config *Config) UpdateUser(req dto.UserUpdate) (dto.UserResponse, bool) {

	db := config.db

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
