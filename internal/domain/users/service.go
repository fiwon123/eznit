package users

import (
	"log/slog"
	"net/http"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/logger"
)

type Service struct {
	db      Repository
	session *sessions.Service
	logger  *logger.Config
}

func NewService(db Repository, session *sessions.Service, logger *logger.Config) *Service {
	return &Service{
		db:      db,
		session: session,
		logger:  logger,
	}
}

func (s *Service) GetUsers() ([]UserData, *errors.AppError) {
	s.logger.Debug("GetUsers")

	usersModel, ok := s.db.GetUsers()
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "can't get users")
	}

	resp := []UserData{}
	for _, user := range usersModel {
		resp = append(resp, UserData{
			Email: user.Email,
		})
	}

	return resp, nil
}

func (s *Service) GetUser(id string) (*UserData, *errors.AppError) {
	s.logger.Debug("GetUser", slog.String("id", id))

	user, ok := s.db.GetUser(id)
	if !ok {
		s.logger.Error("User not found")
		return nil, errors.NewAppError(http.StatusNotFound, "user not found")
	}

	resp := &UserData{
		Email: user.Email,
	}

	return resp, nil
}

func (s *Service) SignupUser(req SignupRequest) (string, *errors.AppError) {
	s.logger.Debug("SignupUser", slog.Any("request", req))

	if req.Password != req.ConfirmPassword {
		s.logger.Error("passwords not match")
		return "", errors.NewAppError(http.StatusBadRequest, "passwords not match")
	}

	if req.Password == "" {
		s.logger.Error("passwords is empty")
		return "", errors.NewAppError(http.StatusBadRequest, "passwords is empty")
	}

	return "user registered!", nil
}

func (s *Service) LoginUser(req LoginRequest) (LoginResponse, *errors.AppError) {
	s.logger.Debug("LoginUser", slog.Any("request", req))

	db := s.db

	user, ok := db.GetUserByEmail(req.Email)
	if !ok {
		s.logger.Error("user not found")
		return LoginResponse{}, errors.NewAppError(http.StatusNotFound, "user not found")
	}

	if !checkPasswordHash(req.Password, user.Password) {
		s.logger.Error("passwords not match")
		return LoginResponse{}, errors.NewAppError(http.StatusBadRequest, "passwords not match")
	}

	s.logger.Debug("user logged in")
	token, ok := s.session.CreateToken(user.ID)
	if !ok {
		s.logger.Error("create token failed")
		return LoginResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to create a new session")
	}

	return LoginResponse{
		Token: token,
	}, nil
}

func (s *Service) CreateUser(req CreateRequest) (string, *errors.AppError) {
	s.logger.Debug("CreateUser", slog.Any("request", req))

	db := s.db

	if db.UserExists(req.Email) {
		s.logger.Warn("user already exists", slog.String("email", req.Email))
		return "", errors.NewAppError(http.StatusConflict, "user already exists")
	}

	if req.Password == "" {
		s.logger.Warn("password is empty")
		return "", errors.NewAppError(http.StatusBadRequest, "password is empty")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	if !db.CreateUser(User{
		Email:    req.Email,
		Password: hash,
	}) {
		s.logger.Error("create user failed")
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	s.logger.Debug("user created!")

	return "user created!", nil
}

func (s *Service) DeleteUser(req DeleteRequest) (string, *errors.AppError) {
	s.logger.Debug("DeleteUser", slog.Any("request", req))

	db := s.db

	user, ok := db.GetUser(req.Id)
	if !ok {
		return "", errors.NewAppError(http.StatusNotFound, "user not exists")
	}

	if !db.DeleteUser(*user) {
		return "", errors.NewAppError(http.StatusNotFound, "delete user failed")
	}

	s.logger.Debug("user deleted!")

	return "user deleted!", nil
}

func (s *Service) UpdateUser(req UpdateRequest) (string, *errors.AppError) {
	s.logger.Debug("UpdateUser", slog.Any("request", req))

	if req.Password == "" {
		s.logger.Error("password is empty")
		return "", errors.NewAppError(http.StatusBadRequest, "password is empty")
	}

	db := s.db

	user, ok := db.GetUser(req.Id)
	if !ok {
		return "", errors.NewAppError(http.StatusNotFound, "user not exists")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to update user")
	}

	user.Email = req.Email
	user.Password = hash

	if !db.UpdateUser(*user) {
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to update user")
	}

	s.logger.Debug("user updated!")

	return "user updated!", nil
}
