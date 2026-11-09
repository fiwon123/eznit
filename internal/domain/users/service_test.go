package users

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	apperrors "github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// fakeRepository is an in-memory Repository. The rules under test live in the
// service, so this is what pins them down without a database.
//
// It proves the rules, not the SQL: a fake cannot see a missing foreign key or
// a column the driver will not scan, which is why schema_test.go checks the
// queries against the migrations too.
type fakeRepository struct {
	users []User

	// readFails makes the account reads (GetUser, GetUserByEmail, GetUsers,
	// UserExists) report a failure that is not "no such row", the way a
	// database that is down would. HasUsers and CommandsInUseByOthers have
	// their own knobs, so a test can fail one question at a time.
	readFails bool

	// writeFails makes the create and the update report a failure.
	writeFails bool

	// updateRaced and deleteRaced make the write answer "not found" for a row
	// the fake still holds: a concurrent delete between the read and the write.
	updateRaced bool
	deleteRaced bool

	// hasUsersFails makes HasUsers report that it could not answer, the way a
	// database that is down would.
	hasUsersFails bool

	// created collects the rows the service asked to insert, so a refusal can
	// be checked to have created nothing.
	created []User

	// commandInUseByOthers is the answer to "does another account's template
	// pull in one of mine?", and commandInUseCheckFails makes the question
	// unanswerable.
	commandInUseByOthers   bool
	commandInUseCheckFails bool

	// deleteFails makes the row delete report failure, the way a database that
	// is down would.
	deleteFails bool

	// ownerStorage is what a successful delete reports for the disk: the
	// template and command ids whose folders the service has to remove.
	ownerStorage OwnerStorage
}

func newFakeRepository(users ...User) *fakeRepository {
	return &fakeRepository{users: users}
}

func (r *fakeRepository) HasUsers() (bool, error) {
	if r.hasUsersFails {
		return false, errDatabaseDown
	}
	return len(r.users) > 0, nil
}

func (r *fakeRepository) GetUsers() ([]User, error) {
	if r.readFails {
		return nil, errDatabaseDown
	}
	return r.users, nil
}

func (r *fakeRepository) GetUser(id uuid.UUID) (*User, error) {
	if r.readFails {
		return nil, errDatabaseDown
	}

	for index := range r.users {
		if r.users[index].ID == id {
			user := r.users[index]
			return &user, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepository) GetUserByEmail(email string) (*User, error) {
	if r.readFails {
		return nil, errDatabaseDown
	}

	for index := range r.users {
		if r.users[index].Email == email {
			user := r.users[index]
			return &user, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepository) UserExists(email string) (bool, error) {
	_, err := r.GetUserByEmail(email)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *fakeRepository) CreateUser(user User) error {
	if r.writeFails {
		return errDatabaseDown
	}

	user.ID = uuid.New()
	r.users = append(r.users, user)
	r.created = append(r.created, user)
	return nil
}

func (r *fakeRepository) DeleteUser(user User) (OwnerStorage, error) {
	if r.deleteFails {
		return OwnerStorage{}, errDatabaseDown
	}

	if r.deleteRaced {
		return OwnerStorage{}, ErrNotFound
	}

	for index := range r.users {
		if r.users[index].ID == user.ID {
			r.users = append(r.users[:index], r.users[index+1:]...)
			return r.ownerStorage, nil
		}
	}

	return OwnerStorage{}, ErrNotFound
}

func (r *fakeRepository) CommandsInUseByOthers(userID uuid.UUID) (bool, error) {
	if r.commandInUseCheckFails {
		return false, errDatabaseDown
	}

	return r.commandInUseByOthers, nil
}

// errDatabaseDown stands in for every way a query can fail without answering:
// it must never be read as "not found".
var errDatabaseDown = errors.New("the database is down")

func (r *fakeRepository) UpdateUser(user User) error {
	if r.writeFails {
		return errDatabaseDown
	}

	if r.updateRaced {
		return ErrNotFound
	}

	for index := range r.users {
		if r.users[index].ID == user.ID {
			r.users[index] = user
			return nil
		}
	}
	return ErrNotFound
}

// newTestService returns a service backed by the given repository and a logger
// that writes to a throwaway folder. The session service stays nil: these tests
// exercise signup and the account rules, and login — the only path that creates
// a token — has its own test through the router.
func newTestService(t *testing.T, repo Repository) *Service {
	t.Helper()

	return newTestServiceAt(t, repo, t.TempDir())
}

// newTestServiceAt is newTestService with a chosen upload root, for the tests
// that check what a delete leaves on disk.
func newTestServiceAt(t *testing.T, repo Repository, uploadFolder string) *Service {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewService(repo, uploadFolder, nil, log)
}

func signup(email string) SignupRequest {
	return SignupRequest{Email: email, Password: "secret", ConfirmPassword: "secret"}
}

// The first account created on an instance is its admin: there is no operator
// to promote one, and without it the admin routes could never be used.
func TestCreateUserMakesTheFirstAccountAnAdmin(t *testing.T) {
	repo := newFakeRepository()

	if _, appError := newTestService(t, repo).CreateUser(context.Background(), signup("first@example.com")); appError != nil {
		t.Fatalf("expected the first signup to succeed, got %v", appError)
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected one account to be created, got %d", len(repo.created))
	}

	if !repo.created[0].IsAdmin {
		t.Error("expected the first account to be an admin")
	}
}

// Every account after it is an ordinary user: the bootstrap is the empty
// table, not the position of the account in the list.
func TestCreateUserDoesNotMakeLaterAccountsAdmins(t *testing.T) {
	repo := newFakeRepository(User{ID: uuid.New(), Email: "first@example.com", IsAdmin: true})

	if _, appError := newTestService(t, repo).CreateUser(context.Background(), signup("second@example.com")); appError != nil {
		t.Fatalf("expected the second signup to succeed, got %v", appError)
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected one account to be created, got %d", len(repo.created))
	}

	if repo.created[0].IsAdmin {
		t.Error("expected the second account not to be an admin")
	}
}

// The bootstrap must not grant admin on a guess: when the repository cannot say
// whether the table is empty, the signup fails instead of creating an admin.
func TestCreateUserRefusesWhenItCannotTellWhetherTheTableIsEmpty(t *testing.T) {
	repo := newFakeRepository()
	repo.hasUsersFails = true

	_, appError := newTestService(t, repo).CreateUser(context.Background(), signup("first@example.com"))
	if appError == nil {
		t.Fatal("expected the signup to fail")
	}

	if appError.StatusCode() != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, appError.StatusCode())
	}

	if len(repo.created) != 0 {
		t.Errorf("expected nothing to be created, got %d accounts", len(repo.created))
	}
}

// A refused signup creates nothing, so a rejected request cannot leave an
// account behind — or an admin.
func TestCreateUserCreatesNothingWhenTheRequestIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		request SignupRequest
		status  int
	}{
		{
			name:    "an empty password",
			request: SignupRequest{Email: "new@example.com", ConfirmPassword: ""},
			status:  http.StatusBadRequest,
		},
		{
			name:    "a password that was not confirmed",
			request: SignupRequest{Email: "new@example.com", Password: "secret", ConfirmPassword: "other"},
			status:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()

			_, appError := newTestService(t, repo).CreateUser(context.Background(), tt.request)
			if appError == nil {
				t.Fatal("expected the signup to fail")
			}

			if appError.StatusCode() != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, appError.StatusCode())
			}

			if len(repo.created) != 0 {
				t.Errorf("expected nothing to be created, got %d accounts", len(repo.created))
			}
		})
	}
}

// The caller reads their own record.
func TestGetUserReturnsTheCallersOwnRecord(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	svc := newTestService(t, newFakeRepository(owner))

	resp, appError := svc.GetUser(context.Background(), owner.ID, owner.ID)
	if appError != nil {
		t.Fatalf("expected the caller's record, got %v", appError)
	}

	if resp.Email != owner.Email {
		t.Errorf("expected %q, got %q", owner.Email, resp.Email)
	}
}

// Another account's id is not readable, and it answers exactly like an id that
// does not exist: a session must not be able to probe for accounts.
func TestGetUserReportsAnotherAccountAsNotFound(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	stranger := User{ID: uuid.New(), Email: "stranger@example.com"}
	svc := newTestService(t, newFakeRepository(owner, stranger))

	_, appError := svc.GetUser(context.Background(), stranger.ID, owner.ID)
	if appError == nil {
		t.Fatal("expected another account to be refused")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// A caller whose own account is gone — a session that outlived its row — gets
// the same 404 as an id that never existed, from the repository lookup rather
// than the caller check.
func TestGetUserReportsAMissingAccountAsNotFound(t *testing.T) {
	owner := uuid.New()
	svc := newTestService(t, newFakeRepository())

	_, appError := svc.GetUser(context.Background(), owner, owner)
	if appError == nil {
		t.Fatal("expected a missing account to be refused")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// The admin question is answered from the column, and an account the service
// cannot read is not an admin.
func TestIsAdminAnswersFromTheAccount(t *testing.T) {
	admin := User{ID: uuid.New(), Email: "admin@example.com", IsAdmin: true}
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	svc := newTestService(t, newFakeRepository(admin, owner))

	tests := []struct {
		name  string
		id    uuid.UUID
		wants bool
	}{
		{"an admin", admin.ID, true},
		{"an ordinary account", owner.ID, false},
		{"an id that does not exist", uuid.New(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.IsAdmin(context.Background(), tt.id); got != tt.wants {
				t.Errorf("IsAdmin(%s) = %t, want %t", tt.id, got, tt.wants)
			}
		})
	}
}

// A signup for an address that already has an account answers exactly like a
// real one, and creates nothing: the response must not say which addresses are
// registered.
func TestSignupDoesNotRevealWhetherAnEmailIsRegistered(t *testing.T) {
	taken := User{ID: uuid.New(), Email: "taken@example.com"}
	repo := newFakeRepository(taken)

	message, appError := newTestService(t, repo).CreateUser(context.Background(), signup(taken.Email))
	if appError != nil {
		t.Fatalf("expected the answer a real signup gets, got %v", appError)
	}

	if message != "user created!" {
		t.Errorf("expected the answer a real signup gets, got %q", message)
	}

	if len(repo.created) != 0 {
		t.Errorf("expected no account to be created, got %d", len(repo.created))
	}
}

// An address with no account and a wrong password get the same 401 and the same
// message: a caller learns that the pair did not work, not which half failed.
func TestLoginDoesNotRevealWhetherAnEmailIsRegistered(t *testing.T) {
	hash, err := hashPassword("right-password")
	if err != nil {
		t.Fatalf("failed to hash the test password: %v", err)
	}

	owner := User{ID: uuid.New(), Email: "owner@example.com", Password: hash}
	svc := newTestService(t, newFakeRepository(owner))

	tests := []struct {
		name    string
		request LoginRequest
	}{
		{"an address with no account", LoginRequest{Email: "nobody@example.com", Password: "whatever"}},
		{"a wrong password", LoginRequest{Email: owner.Email, Password: "wrong"}},
	}

	var first string
	for index, tt := range tests {
		_, appError := svc.LoginUser(context.Background(), tt.request)
		if appError == nil {
			t.Fatalf("%s: expected the login to fail", tt.name)
		}

		if appError.StatusCode() != http.StatusUnauthorized {
			t.Errorf("%s: expected status %d, got %d", tt.name, http.StatusUnauthorized, appError.StatusCode())
		}

		if index == 0 {
			first = appError.Error()
			continue
		}

		if appError.Error() != first {
			t.Errorf("the two failures answer differently: %q and %q", first, appError.Error())
		}
	}
}

// The right pair still gets a token: closing the oracle must not close the door.
func TestLoginAnswersWithAToken(t *testing.T) {
	hash, err := hashPassword("right-password")
	if err != nil {
		t.Fatalf("failed to hash the test password: %v", err)
	}

	owner := User{ID: uuid.New(), Email: "owner@example.com", Password: hash}

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	svc := NewService(newFakeRepository(owner), t.TempDir(), sessions.NewService(newFakeSessionsRepository(), log), log)

	resp, appError := svc.LoginUser(context.Background(), LoginRequest{Email: owner.Email, Password: "right-password"})
	if appError != nil {
		t.Fatalf("expected the login to succeed, got %v", appError)
	}

	if resp.Token == "" {
		t.Error("expected a token")
	}
}

// writeFolder creates a folder with one file in it, standing in for the bytes
// an account left on disk.
func writeFolder(t *testing.T, folder string) {
	t.Helper()

	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("failed to create %s: %v", folder, err)
	}

	if err := os.WriteFile(filepath.Join(folder, "stored"), []byte("bytes"), 0o600); err != nil {
		t.Fatalf("failed to write into %s: %v", folder, err)
	}
}

// Deleting an account removes the rows and the bytes: the account's own upload
// folder, and the binary folders of the templates and commands it owned.
func TestDeleteUserRemovesTheAccountAndItsBytes(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	repo := newFakeRepository(owner)

	templateID := uuid.New()
	commandID := uuid.New()
	repo.ownerStorage = OwnerStorage{
		TemplateIDs: []uuid.UUID{templateID},
		CommandIDs:  []uuid.UUID{commandID},
	}

	uploadFolder := t.TempDir()
	folders := []string{
		filepath.Join(uploadFolder, owner.ID.String()),
		filepath.Join(uploadFolder, "templates", templateID.String()),
		filepath.Join(uploadFolder, "commands", commandID.String()),
	}
	for _, folder := range folders {
		writeFolder(t, folder)
	}

	if _, appError := newTestServiceAt(t, repo, uploadFolder).DeleteUser(context.Background(), DeleteRequest{Id: owner.ID}, uuid.New()); appError != nil {
		t.Fatalf("expected the account to be deleted, got %v", appError)
	}

	if _, err := repo.GetUser(owner.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the account is still in the repository: %v", err)
	}

	for _, folder := range folders {
		if _, err := os.Stat(folder); !os.IsNotExist(err) {
			t.Errorf("expected %s to be gone, stat returned %v", folder, err)
		}
	}

	if _, err := os.Stat(uploadFolder); err != nil {
		t.Errorf("the delete removed the upload root itself: %v", err)
	}
}

// A command still referenced by a template refuses the delete: the schema keeps
// that reference RESTRICT so a published template does not quietly lose a
// script. The API only lets a template reference its owner's commands, so this
// state comes from data written outside it — the pre-check turns a schema
// refusal into a 409 rather than a failed transaction.
func TestDeleteUserRefusesWhenAnotherAccountsTemplateUsesACommand(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	repo := newFakeRepository(owner)
	repo.commandInUseByOthers = true

	uploadFolder := t.TempDir()
	userFolder := filepath.Join(uploadFolder, owner.ID.String())
	writeFolder(t, userFolder)

	_, appError := newTestServiceAt(t, repo, uploadFolder).DeleteUser(context.Background(), DeleteRequest{Id: owner.ID}, uuid.New())
	if appError == nil {
		t.Fatal("expected the delete to be refused")
	}

	if appError.StatusCode() != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, appError.StatusCode())
	}

	if _, err := repo.GetUser(owner.ID); err != nil {
		t.Errorf("the refused delete removed the account: %v", err)
	}

	if _, err := os.Stat(userFolder); err != nil {
		t.Errorf("the refused delete removed bytes: %v", err)
	}
}

// An admin deleting the account it is signed in with is refused: the request
// would drop its own session, and on an instance with a single admin it would
// leave nobody able to reach the admin routes.
func TestDeleteUserRefusesToDeleteTheSignedInAccount(t *testing.T) {
	admin := User{ID: uuid.New(), Email: "admin@example.com", IsAdmin: true}
	repo := newFakeRepository(admin)

	_, appError := newTestService(t, repo).DeleteUser(context.Background(), DeleteRequest{Id: admin.ID}, admin.ID)
	if appError == nil {
		t.Fatal("expected the self-delete to be refused")
	}

	if appError.StatusCode() != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, appError.StatusCode())
	}

	if _, err := repo.GetUser(admin.ID); err != nil {
		t.Errorf("the refused delete removed the account: %v", err)
	}
}

// A database failure is not "the account does not exist": the route answers
// 500, which is what #144 was filed about.
func TestDeleteUserReportsAFailedDeleteAsServerError(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	repo := newFakeRepository(owner)
	repo.deleteFails = true

	_, appError := newTestService(t, repo).DeleteUser(context.Background(), DeleteRequest{Id: owner.ID}, uuid.New())
	if appError == nil {
		t.Fatal("expected the delete to fail")
	}

	if appError.StatusCode() != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, appError.StatusCode())
	}
}

// A check that cannot be answered is a 500 too: the delete must not go ahead
// on a guess about what still references the account.
func TestDeleteUserReportsAFailedCommandCheckAsServerError(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	repo := newFakeRepository(owner)
	repo.commandInUseCheckFails = true

	_, appError := newTestService(t, repo).DeleteUser(context.Background(), DeleteRequest{Id: owner.ID}, uuid.New())
	if appError == nil {
		t.Fatal("expected the delete to fail")
	}

	if appError.StatusCode() != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, appError.StatusCode())
	}

	if _, err := repo.GetUser(owner.ID); err != nil {
		t.Errorf("the delete went ahead without an answer: %v", err)
	}
}

// A missing account is a 404 — and only that is.
func TestDeleteUserReportsAMissingAccountAsNotFound(t *testing.T) {
	_, appError := newTestService(t, newFakeRepository()).DeleteUser(context.Background(), DeleteRequest{Id: uuid.New()}, uuid.New())
	if appError == nil {
		t.Fatal("expected the delete to be refused")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// A read that fails is not "the account does not exist". Every route that reads
// an account before acting on it has to keep the two apart (#148).
func TestAFailedReadIsNotNotFound(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}

	tests := []struct {
		name string
		call func(*Service) *apperrors.AppError
	}{
		{
			name: "the caller's own record",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.GetUser(context.Background(), owner.ID, owner.ID)
				return appError
			},
		},
		{
			name: "a login",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.LoginUser(context.Background(), LoginRequest{Email: owner.Email, Password: "whatever"})
				return appError
			},
		},
		{
			name: "a delete",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.DeleteUser(context.Background(), DeleteRequest{Id: owner.ID}, uuid.New())
				return appError
			},
		},
		{
			name: "an update",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.UpdateUser(context.Background(), UpdateRequest{Id: owner.ID, Email: owner.Email, Password: "secret"})
				return appError
			},
		},
		{
			name: "a signup",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.CreateUser(context.Background(), signup(owner.Email))
				return appError
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository(owner)
			repo.readFails = true

			appError := tt.call(newTestService(t, repo))
			if appError == nil {
				t.Fatal("expected the read failure to be reported")
			}

			if appError.StatusCode() != http.StatusInternalServerError {
				t.Errorf("expected status %d, got %d (%s)", http.StatusInternalServerError, appError.StatusCode(), appError.Error())
			}
		})
	}
}

// A write that fails is a 500: the account was not created or changed, and the
// answer must not claim it was.
func TestAFailedWriteIsAServerError(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}

	tests := []struct {
		name string
		call func(*Service) *apperrors.AppError
	}{
		{
			name: "a signup",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.CreateUser(context.Background(), signup("new@example.com"))
				return appError
			},
		},
		{
			name: "an update",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.UpdateUser(context.Background(), UpdateRequest{Id: owner.ID, Email: owner.Email, Password: "secret"})
				return appError
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository(owner)
			repo.writeFails = true

			appError := tt.call(newTestService(t, repo))
			if appError == nil {
				t.Fatal("expected the write failure to be reported")
			}

			if appError.StatusCode() != http.StatusInternalServerError {
				t.Errorf("expected status %d, got %d (%s)", http.StatusInternalServerError, appError.StatusCode(), appError.Error())
			}
		})
	}
}

// A write that finds the row gone — another request deleted it between the read
// and the write — is a 404, not a failure and not a success.
func TestAWriteThatRacedADeleteIsNotFound(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}

	tests := []struct {
		name string
		call func(*Service) *apperrors.AppError
		race func(*fakeRepository)
	}{
		{
			name: "an update",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.UpdateUser(context.Background(), UpdateRequest{Id: owner.ID, Email: owner.Email, Password: "secret"})
				return appError
			},
			race: func(repo *fakeRepository) { repo.updateRaced = true },
		},
		{
			name: "a delete",
			call: func(s *Service) *apperrors.AppError {
				_, appError := s.DeleteUser(context.Background(), DeleteRequest{Id: owner.ID}, uuid.New())
				return appError
			},
			race: func(repo *fakeRepository) { repo.deleteRaced = true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository(owner)
			tt.race(repo)

			appError := tt.call(newTestService(t, repo))
			if appError == nil {
				t.Fatal("expected the race to be reported")
			}

			if appError.StatusCode() != http.StatusNotFound {
				t.Errorf("expected status %d, got %d (%s)", http.StatusNotFound, appError.StatusCode(), appError.Error())
			}
		})
	}
}
