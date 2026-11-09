package users

import (
	"context"
	"net/http"
	"testing"

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

	// hasUsersFails makes HasUsers report that it could not answer, the way a
	// database that is down would.
	hasUsersFails bool

	// created collects the rows the service asked to insert, so a refusal can
	// be checked to have created nothing.
	created []User
}

func newFakeRepository(users ...User) *fakeRepository {
	return &fakeRepository{users: users}
}

func (r *fakeRepository) HasUsers() (bool, bool) {
	if r.hasUsersFails {
		return false, false
	}
	return len(r.users) > 0, true
}

func (r *fakeRepository) GetUsers() ([]User, bool) {
	return r.users, true
}

func (r *fakeRepository) GetUser(id uuid.UUID) (*User, bool) {
	for index := range r.users {
		if r.users[index].ID == id {
			user := r.users[index]
			return &user, true
		}
	}
	return nil, false
}

func (r *fakeRepository) GetUserByEmail(email string) (*User, bool) {
	for index := range r.users {
		if r.users[index].Email == email {
			user := r.users[index]
			return &user, true
		}
	}
	return nil, false
}

func (r *fakeRepository) UserExists(email string) bool {
	_, ok := r.GetUserByEmail(email)
	return ok
}

func (r *fakeRepository) CreateUser(user User) bool {
	user.ID = uuid.New()
	r.users = append(r.users, user)
	r.created = append(r.created, user)
	return true
}

func (r *fakeRepository) DeleteUser(user User) bool {
	for index := range r.users {
		if r.users[index].ID == user.ID {
			r.users = append(r.users[:index], r.users[index+1:]...)
			return true
		}
	}
	return false
}

func (r *fakeRepository) UpdateUser(user User) bool {
	for index := range r.users {
		if r.users[index].ID == user.ID {
			r.users[index] = user
			return true
		}
	}
	return false
}

// newTestService returns a service backed by the given repository and a logger
// that writes to a throwaway folder. The session service stays nil: these tests
// exercise signup and the account rules, and login — the only path that creates
// a token — has its own test through the router.
func newTestService(t *testing.T, repo Repository) *Service {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewService(repo, nil, log)
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
