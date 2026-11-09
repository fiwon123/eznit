package users

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// User Service handle all incoming request passed by handlers
// and calling other necessary services or repositories to complete request
type Service struct {
	db Repository
	// uploadFolder is the root the account's bytes live under: the files at
	// `<upload-root>/<user-id>/`, the template binaries at
	// `<upload-root>/templates/<template-id>/` and the command binaries at
	// `<upload-root>/commands/<command-id>/`. Deleting an account removes them.
	uploadFolder string
	session      *sessions.Service
	logger       *logger.Config
}

// Return a new User Service
func NewService(db Repository, uploadFolder string, session *sessions.Service, logger *logger.Config) *Service {
	return &Service{
		db:           db,
		uploadFolder: uploadFolder,
		session:      session,
		logger:       logger,
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
//
// An account is deleted as a whole: its sessions, files (and their history),
// templates and commands go with it, and so do the bytes those rows named.
//
// Two deletes are refused. The account the caller is signed in with is a 409:
// the request would drop its own session, and on an instance with one admin it
// would leave nobody able to reach the admin routes. And an account whose
// command another template still pulls in is a 409 too — the schema keeps that
// reference RESTRICT so a published template does not quietly lose a script,
// which is the rule commands.DeleteCommand already follows. A database failure
// is a 500, never the 404 that used to be answered for every failure (#144).
func (s *Service) DeleteUser(ctx context.Context, req DeleteRequest, callerID uuid.UUID) (string, *errors.AppError) {
	s.logger.Debug("DeleteUser", slog.String("id", req.Id.String()))

	if req.Id == callerID {
		s.logger.Warn("the caller tried to delete the account it is signed in with", slog.String("id", req.Id.String()))
		return "", errors.NewAppError(http.StatusConflict, "you cannot delete the account you are signed in with")
	}

	db := s.db

	user, ok := db.GetUser(req.Id)
	if !ok {
		return "", errors.NewAppError(http.StatusNotFound, "user not exists")
	}

	inUse, ok := db.CommandsInUseByOthers(req.Id)
	if !ok {
		s.logger.Error("failed to check whether a template uses the account's commands")
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to delete user")
	}

	if inUse {
		s.logger.Warn("a template of another account pulls in one of this account's commands")
		return "", errors.NewAppError(http.StatusConflict, "a template still uses a command owned by this account")
	}

	owner, ok := db.DeleteUser(*user)
	if !ok {
		s.logger.Error("delete user failed")
		return "", errors.NewAppError(http.StatusInternalServerError, "failed to delete user")
	}

	s.removeAccountStorage(req.Id, owner)

	s.logger.Debug("user deleted!")

	return "user deleted!", nil
}

// removeAccountStorage removes the bytes a deleted account left behind: its
// uploads, and the binary files of its templates and commands.
//
// The rows are already gone, so a folder that will not go is a leak, not a
// failure the caller can act on — it is logged and the delete stands, the way
// the templates and commands domains already handle their own storage.
func (s *Service) removeAccountStorage(userID uuid.UUID, owner OwnerStorage) {
	folders := []string{filepath.Join(s.uploadFolder, userID.String())}

	for _, id := range owner.TemplateIDs {
		folders = append(folders, filepath.Join(s.uploadFolder, "templates", id.String()))
	}

	for _, id := range owner.CommandIDs {
		folders = append(folders, filepath.Join(s.uploadFolder, "commands", id.String()))
	}

	for _, folder := range folders {
		// The folder is built from ids the database returned, but it is still
		// checked: an empty upload root or a path that escaped it must not turn
		// a delete into a recursive removal somewhere else.
		if !helper.PathIsInside(s.uploadFolder, folder) {
			s.logger.Error("refusing to remove a folder outside the upload folder", slog.String("path", folder))
			continue
		}

		if err := os.RemoveAll(folder); err != nil {
			s.logger.Warn("failed to remove the account's storage", slog.String("path", folder), slog.Any("error", err))
		}
	}
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
