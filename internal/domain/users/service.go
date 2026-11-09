package users

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	apperrors "github.com/fiwon123/eznit/pkg/errors"
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
func (s *Service) GetUsers(ctx context.Context) ([]UserData, *apperrors.AppError) {
	s.logger.Debug("GetUsers")

	usersModel, err := s.db.GetUsers()
	if err != nil {
		s.logger.Error("get users failed", slog.Any("error", err))
		return nil, apperrors.NewAppError(http.StatusInternalServerError, "can't get users")
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
func (s *Service) GetUser(ctx context.Context, id, callerID uuid.UUID) (*UserData, *apperrors.AppError) {
	s.logger.Debug("GetUser", slog.String("id", id.String()))

	if id != callerID {
		s.logger.Warn("user is not the caller", slog.String("id", id.String()))
		return nil, apperrors.NewAppError(http.StatusNotFound, "user not found")
	}

	user, err := s.db.GetUser(id)
	if errors.Is(err, ErrNotFound) {
		s.logger.Error("User not found")
		return nil, apperrors.NewAppError(http.StatusNotFound, "user not found")
	}
	if err != nil {
		s.logger.Error("get user failed", slog.Any("error", err))
		return nil, apperrors.NewAppError(http.StatusInternalServerError, "failed to read the user")
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
	user, err := s.db.GetUser(id)
	if err != nil {
		s.logger.Warn("admin check on an unreadable account", slog.String("id", id.String()), slog.Any("error", err))
		return false
	}

	return user.IsAdmin
}

// Check if user credentials are valid
//
// Both ways a login can fail answer the same thing. Login is public, and a
// difference between "no such address" and "wrong password" tells a caller
// which addresses have accounts; the server log keeps the distinction.
func (s *Service) LoginUser(ctx context.Context, req LoginRequest) (LoginResponse, *apperrors.AppError) {
	s.logger.Debug("LoginUser", slog.String("email", req.Email))

	db := s.db

	user, err := db.GetUserByEmail(req.Email)
	if errors.Is(err, ErrNotFound) {
		s.logger.Warn("login for an address with no account", slog.String("email", req.Email))
		return LoginResponse{}, invalidCredentials()
	}
	if err != nil {
		// A read that failed is not a bad password: the caller may retry, and
		// the answer must not say the credentials were checked.
		s.logger.Error("login read failed", slog.Any("error", err))
		return LoginResponse{}, apperrors.NewAppError(http.StatusInternalServerError, "failed to log in")
	}

	if !checkPasswordHash(req.Password, user.Password) {
		s.logger.Warn("login with a wrong password", slog.String("email", req.Email))
		return LoginResponse{}, invalidCredentials()
	}

	s.logger.Debug("user logged in")
	token, ok := s.session.CreateToken(user.ID)
	if !ok {
		s.logger.Error("create token failed")
		return LoginResponse{}, apperrors.NewAppError(http.StatusInternalServerError, "failed to create a new session")
	}

	s.logger.Debug("user token", slog.String("token_fp", helper.Fingerprint(token)))
	return LoginResponse{
		Token: token,
	}, nil
}

// invalidCredentials is the one answer a failed login gets, whatever failed:
// the caller learns that the pair did not work, not which half was wrong.
func invalidCredentials() *apperrors.AppError {
	return apperrors.NewAppError(http.StatusUnauthorized, "invalid credentials")
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
func (s *Service) CreateUser(ctx context.Context, req SignupRequest) (string, *apperrors.AppError) {
	s.logger.Debug("CreateUser", slog.String("email", req.Email))

	db := s.db

	if req.Password == "" {
		s.logger.Warn("password is empty")
		return "", apperrors.NewAppError(http.StatusBadRequest, "password is empty")
	}

	if req.Password != req.ConfirmPassword {
		s.logger.Warn("passwords do not match")
		return "", apperrors.NewAppError(http.StatusBadRequest, "passwords do not match")
	}

	taken, err := db.UserExists(req.Email)
	if err != nil {
		s.logger.Error("can't check whether the address has an account", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	if taken {
		// Signup is public, and "user already exists" tells a caller which
		// addresses have accounts. The answer is the same as a real signup,
		// and the hash is computed anyway so the two paths take about as long.
		// The log keeps the truth for the operator.
		if _, err := hashPassword(req.Password); err != nil {
			s.logger.Error("password error", slog.Any("error", err))
			return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to create new user")
		}

		s.logger.Info("signup for an address that already has an account", slog.String("email", req.Email))
		return "user created!", nil
	}

	hasUsers, err := db.HasUsers()
	if err != nil {
		s.logger.Error("can't tell whether an account exists yet", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to create new user")
	}

	if err := db.CreateUser(User{
		Email:    req.Email,
		Password: hash,
		IsAdmin:  !hasUsers,
	}); err != nil {
		s.logger.Error("create user failed", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to create new user")
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
func (s *Service) DeleteUser(ctx context.Context, req DeleteRequest, callerID uuid.UUID) (string, *apperrors.AppError) {
	s.logger.Debug("DeleteUser", slog.String("id", req.Id.String()))

	if req.Id == callerID {
		s.logger.Warn("the caller tried to delete the account it is signed in with", slog.String("id", req.Id.String()))
		return "", apperrors.NewAppError(http.StatusConflict, "you cannot delete the account you are signed in with")
	}

	db := s.db

	user, err := db.GetUser(req.Id)
	if errors.Is(err, ErrNotFound) {
		return "", apperrors.NewAppError(http.StatusNotFound, "user not exists")
	}
	if err != nil {
		s.logger.Error("read the account to delete", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to delete user")
	}

	inUse, err := db.CommandsInUseByOthers(req.Id)
	if err != nil {
		s.logger.Error("failed to check whether a template uses the account's commands", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to delete user")
	}

	if inUse {
		s.logger.Warn("a template of another account pulls in one of this account's commands")
		return "", apperrors.NewAppError(http.StatusConflict, "a template still uses a command owned by this account")
	}

	owner, err := db.DeleteUser(*user)
	if errors.Is(err, ErrNotFound) {
		// It was read a moment ago: another request deleted it first.
		return "", apperrors.NewAppError(http.StatusNotFound, "user not exists")
	}
	if err != nil {
		s.logger.Error("delete user failed", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to delete user")
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
func (s *Service) UpdateUser(ctx context.Context, req UpdateRequest) (string, *apperrors.AppError) {
	s.logger.Debug("UpdateUser", slog.String("id", req.Id.String()))

	if req.Password == "" {
		s.logger.Error("password is empty")
		return "", apperrors.NewAppError(http.StatusBadRequest, "password is empty")
	}

	db := s.db

	user, err := db.GetUser(req.Id)
	if errors.Is(err, ErrNotFound) {
		return "", apperrors.NewAppError(http.StatusNotFound, "user not exists")
	}
	if err != nil {
		s.logger.Error("read the account to update", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to update user")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		s.logger.Error("password error", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to update user")
	}

	user.Email = req.Email
	user.Password = hash

	err = db.UpdateUser(*user)
	if errors.Is(err, ErrNotFound) {
		// It was read a moment ago: another request deleted it first.
		return "", apperrors.NewAppError(http.StatusNotFound, "user not exists")
	}
	if err != nil {
		s.logger.Error("update user failed", slog.Any("error", err))
		return "", apperrors.NewAppError(http.StatusInternalServerError, "failed to update user")
	}

	s.logger.Debug("user updated!")

	return "user updated!", nil
}
