package users

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// User Service handle all incoming request passed by handlers
// and calling other necessary services or repositories to complete request
type Service struct {
	db      Repository
	session *sessions.Service
	logger  *logger.Config
}

// Return a new User Service
func NewService(db Repository, session *sessions.Service, logger *logger.Config) *Service {
	return &Service{
		db:      db,
		session: session,
		logger:  logger,
	}
}

// Return a user list containing all registered users data
func (s *Service) GetUsers(ctx context.Context) ([]UserData, *errors.AppError) {
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

// Return only a user based in the user identity
//
// The record is the caller's own: any other id answers "not found" exactly
// like an id that does not exist, because whether an account exists is not
// something a session gets to probe.
func (s *Service) GetUser(ctx context.Context, id, callerID uuid.UUID) (*UserData, *errors.AppError) {
	s.logger.Debug("GetUser", slog.String("id", id.String()))

	if id != callerID {
		s.logger.Warn("user is not the caller", slog.String("id", id.String()))
		return nil, errors.NewAppError(http.StatusNotFound, "user not found")
	}

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

// IsAdmin reports whether a user may use the admin routes.
//
// A lookup that fails is a refusal, never a grant: the guard in front of the
// admin routes answers 403 when this answers false, so an account the service
// cannot read is exactly as powerless as one that is not an admin.
func (s *Service) IsAdmin(ctx context.Context, id uuid.UUID) bool {
	user, ok := s.db.GetUser(id)
	if !ok {
		s.logger.Warn("admin check on an unreadable account", slog.String("id", id.String()))
		return false
	}

	return user.IsAdmin
}

// Check if user credentials are valid
func (s *Service) LoginUser(ctx context.Context, req LoginRequest) (LoginResponse, *errors.AppError) {
	s.logger.Debug("LoginUser", slog.String("email", req.Email))

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

	s.logger.Debug("user token", slog.String("token_fp", helper.Fingerprint(token)))
	return LoginResponse{
		Token: token,
	}, nil
}

// Create a new user based on signup request
//
// The account created while the table is empty is the instance's admin: a
// self-hosted install has no operator to promote one, and an admin account
// nobody can create would leave the admin routes unreachable forever.
//
// The check and the insert are two statements, so two signups racing on an
// empty table can both read "no accounts yet" and both become admins. The
// window is one fresh instance wide, and closing it needs a transaction this
// service does not own; it is a known limitation, not an accident.
func (s *Service) CreateUser(ctx context.Context, req SignupRequest) (string, *errors.AppError) {
	s.logger.Debug("CreateUser", slog.String("email", req.Email))

	db := s.db

	if db.UserExists(req.Email) {
		s.logger.Warn("user already exists", slog.String("email", req.Email))
		return "", errors.NewAppError(http.StatusConflict, "user already exists")
	}

	if req.Password == "" {
		s.logger.Warn("password is empty")
		return "", errors.NewAppError(http.StatusBadRequest, "password is empty")
	}

	if req.Password != req.ConfirmPassword {
		s.logger.Warn("passwords do not match")
		return "", errors.NewAppError(http.StatusBadRequest, "passwords do not match")
	}

	hasUsers, ok := db.HasUsers()
	if !ok {
		s.logger.Error("can't tell whether an account exists yet")
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	if !db.CreateUser(User{
		Email:    req.Email,
		Password: hash,
		IsAdmin:  !hasUsers,
	}) {
		s.logger.Error("create user failed")
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	s.logger.Debug("user created!")

	return "user created!", nil
}

// Delete User based on delete request
func (s *Service) DeleteUser(ctx context.Context, req DeleteRequest) (string, *errors.AppError) {
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

// Update user based on update request
func (s *Service) UpdateUser(ctx context.Context, req UpdateRequest) (string, *errors.AppError) {
	s.logger.Debug("UpdateUser", slog.String("id", req.Id.String()))

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
