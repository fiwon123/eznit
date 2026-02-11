package service

import (
	"github.com/fiwon123/eznit/internal/app/dto"
	"github.com/fiwon123/eznit/internal/domain/model"
)

func (config *Config) GetUsers() []dto.UserDataResponse {

	usersModel := config.db.GetUsers()

	resp := []dto.UserDataResponse{}
	for _, user := range usersModel {
		resp = append(resp, dto.UserDataResponse{
			Email: user.Email,
		})
	}

	return resp
}

func (config *Config) GetUser(id string) (dto.UserDataResponse, bool) {

	user := config.db.GetUser(id)
	if user == nil {
		return dto.UserDataResponse{}, false
	}

	resp := dto.UserDataResponse{
		Email: user.Email,
	}

	return resp, true
}

func (config *Config) CreateUser(req dto.UserCreate) (dto.UserMsgResponse, bool) {

	db := config.db

	if db.UserExists(req.Email) {
		return dto.UserMsgResponse{
			Msg: "user already exists",
		}, false
	}

	if !db.CreateUser(model.User{
		Email:    req.Email,
		Password: req.Password,
	}) {
		return dto.UserMsgResponse{
			Msg: "internal server error",
		}, false
	}

	return dto.UserMsgResponse{
		Msg: "user created!",
	}, true
}

func (config *Config) DeleteUser(req dto.UserDelete) (dto.UserMsgResponse, bool) {

	db := config.db

	user := db.GetUser(req.Id)
	if user == nil {
		return dto.UserMsgResponse{
			Msg: "user not exists",
		}, false
	}

	if !db.DeleteUser(*user) {
		return dto.UserMsgResponse{
			Msg: "internal server error",
		}, false
	}

	return dto.UserMsgResponse{
		Msg: "user deleted!",
	}, true
}

func (config *Config) UpdateUser(req dto.UserUpdate) (dto.UserMsgResponse, bool) {

	db := config.db

	user := db.GetUser(req.Id)
	if user == nil {
		return dto.UserMsgResponse{
			Msg: "user not exists",
		}, false
	}

	user.Email = req.Email
	user.Password = req.Password

	if !db.UpdateUser(*user) {
		return dto.UserMsgResponse{
			Msg: "internal server error",
		}, false
	}

	return dto.UserMsgResponse{
		Msg: "user updated!",
	}, true
}
