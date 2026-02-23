package users

import (
	"log/slog"

	"github.com/fiwon123/eznit/internal/domain/sessions"
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

func (s *Service) GetUsers() []DataResponse {
	s.logger.Debug("GetUsers")

	usersModel := s.db.GetUsers()

	resp := []DataResponse{}
	for _, user := range usersModel {
		resp = append(resp, DataResponse{
			Email: user.Email,
		})
	}

	return resp
}

func (s *Service) GetUser(id string) (DataResponse, bool) {
	s.logger.Debug("GetUser", slog.String("id", id))

	user := s.db.GetUser(id)
	if user == nil {
		s.logger.Error("User not found")
		return DataResponse{}, false
	}

	resp := DataResponse{
		Email: user.Email,
	}

	return resp, true
}

func (s *Service) SignupUser(req SignupRequest) (MsgResponse, bool) {
	s.logger.Debug("SignupUser", slog.Any("request", req))

	if req.Password != req.ConfirmPassword {
		s.logger.Error("passwords not match")
		return MsgResponse{
			Msg: "passwords not match",
		}, false
	}

	if req.Password == "" {
		s.logger.Error("passwords is empty")
		return MsgResponse{
			Msg: "passwords is empty",
		}, false
	}

	return s.CreateUser(CreateRequest{
		Email:    req.Email,
		Password: req.Password,
	})
}

func (s *Service) LoginUser(req LoginRequest) (LoginResponse, bool) {
	s.logger.Debug("LoginUser", slog.Any("request", req))

	db := s.db

	user := db.GetUserByEmail(req.Email)
	if user == nil {
		s.logger.Error("user not found")
		return LoginResponse{}, false
	}

	if !checkPasswordHash(req.Password, user.Password) {
		s.logger.Error("passwords not match")
		return LoginResponse{}, false
	}

	s.logger.Debug("user logged in")
	token, ok := s.session.CreateToken(user.ID)
	if !ok {
		s.logger.Error("create token failed")
		return LoginResponse{}, false
	}

	return LoginResponse{
		Token: token,
	}, true
}

func (s *Service) CreateUser(req CreateRequest) (MsgResponse, bool) {
	s.logger.Debug("CreateUser", slog.Any("request", req))

	db := s.db

	if db.UserExists(req.Email) {
		s.logger.Error("user already exists", slog.String("email", req.Email))
		return MsgResponse{
			Msg: "user already exists",
		}, false
	}

	if req.Password == "" {
		s.logger.Error("password is empty")
		return MsgResponse{
			Msg: "password is empty",
		}, false
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	if !db.CreateUser(User{
		Email:    req.Email,
		Password: hash,
	}) {
		s.logger.Error("create user failed")
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	s.logger.Debug("user created!")

	return MsgResponse{
		Msg: "user created!",
	}, true
}

func (s *Service) DeleteUser(req DeleteRequest) (MsgResponse, bool) {
	s.logger.Debug("DeleteUser", slog.Any("request", req))

	db := s.db

	user := db.GetUser(req.Id)
	if user == nil {
		s.logger.Error("user not found")
		return MsgResponse{
			Msg: "user not exists",
		}, false
	}

	if !db.DeleteUser(*user) {
		s.logger.Error("delete user failed")
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	s.logger.Debug("user deleted!")

	return MsgResponse{
		Msg: "user deleted!",
	}, true
}

func (s *Service) UpdateUser(req UpdateRequest) (MsgResponse, bool) {
	s.logger.Debug("UpdateUser", slog.Any("request", req))

	db := s.db

	user := db.GetUser(req.Id)
	if user == nil {
		s.logger.Error("user not found")
		return MsgResponse{
			Msg: "user not exists",
		}, false
	}

	if req.Password == "" {
		s.logger.Error("password is empty")
		return MsgResponse{
			Msg: "password is empty",
		}, false
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	user.Email = req.Email
	user.Password = hash

	if !db.UpdateUser(*user) {
		s.logger.Error("update user failed")
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	s.logger.Debug("user updated!")

	return MsgResponse{
		Msg: "user updated!",
	}, true
}
