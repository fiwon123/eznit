package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// fakeRepository is a scriptable sessions.Repository: it returns whatever err it
// is given, so a test can drive the service's error mapping.
type fakeRepository struct {
	session *Session
	err     error
	created bool
	updated bool
}

func (r *fakeRepository) GetSession(string) (*Session, error)            { return r.session, r.err }
func (r *fakeRepository) GetSessionByUserID(uuid.UUID) (*Session, error) { return r.session, r.err }
func (r *fakeRepository) GetUserIDByToken(string) (uuid.UUID, error) {
	if r.err != nil {
		return uuid.UUID{}, r.err
	}
	return uuid.New(), nil
}
func (r *fakeRepository) CreateSession(Session) error {
	if r.err != nil {
		return r.err
	}
	r.created = true
	return nil
}
func (r *fakeRepository) UpdateSession(Session) error {
	if r.err != nil {
		return r.err
	}
	r.updated = true
	return nil
}

func newTestService(t *testing.T, repo Repository) *Service {
	t.Helper()
	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return NewService(repo, log)
}

func TestIsValidAnswersFalseOnAnyRepositoryError(t *testing.T) {
	for _, repoErr := range []error{ErrNotFound, errors.New("database down")} {
		if newTestService(t, &fakeRepository{err: repoErr}).IsValid("token") {
			t.Fatalf("IsValid with %v = true, want false", repoErr)
		}
	}
}

func TestIsValidAcceptsALiveSession(t *testing.T) {
	svc := newTestService(t, &fakeRepository{session: &Session{IsActive: true, ExpiresAt: time.Now().Add(time.Hour)}})
	if !svc.IsValid("token") {
		t.Fatal("a live session must be valid")
	}
}

func TestCreateTokenAnswersFalseWhenTheWriteFails(t *testing.T) {
	svc := newTestService(t, &fakeRepository{err: errors.New("write failed")})
	if _, ok := svc.CreateToken(uuid.New()); ok {
		t.Fatal("CreateToken should fail when the repository errors")
	}
}

func TestUseTokenAnswersFalseWhenTheWriteFails(t *testing.T) {
	svc := newTestService(t, &fakeRepository{err: errors.New("write failed")})
	if svc.UseToken(context.Background(), "token") {
		t.Fatal("UseToken should fail when the repository errors")
	}
}

func TestGetTokenAnswersEmptyOnError(t *testing.T) {
	svc := newTestService(t, &fakeRepository{err: ErrNotFound})
	if got := svc.GetToken(context.Background(), uuid.New()); got.Token != "" {
		t.Fatalf("GetToken = %q, want empty", got.Token)
	}
}

func TestGetUserIDByTokenAnswersFalseOnError(t *testing.T) {
	svc := newTestService(t, &fakeRepository{err: ErrNotFound})
	if _, ok := svc.GetUserIDByToken("token"); ok {
		t.Fatal("GetUserIDByToken should fail when the repository errors")
	}
}
