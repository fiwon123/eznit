package users

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/internal/platform/ratelimit"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
)

func newRateLimitedRouter(t *testing.T, policy RateLimitPolicy) *chi.Mux {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	sessionsService := sessions.NewService(newFakeSessionsRepository(), log)
	service := NewService(newFakeRepository(), t.TempDir(), sessionsService, log)
	guard := middleware.NewGuard(sessionsService, service, log)

	handler := NewHandler(service, sessionsService, guard, log, policy)

	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	return router
}

func frozenLimiter() *ratelimit.Limiter {
	return ratelimit.NewWithClock(time.Minute, func() time.Time { return time.Unix(1_700_000_000, 0) })
}

func TestLoginIsRateLimitedPerAddress(t *testing.T) {
	router := newRateLimitedRouter(t, RateLimitPolicy{
		Limiter:       frozenLimiter(),
		LoginPerIP:    100,
		LoginPerEmail: 2,
	})
	body := `{"email":"a@example.com","password":"wrong"}`

	for i := 0; i < 2; i++ {
		rec := do(t, router, http.MethodPost, "/v1/login", "", body)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d should not be limited", i+1)
		}
	}

	rec := do(t, router, http.MethodPost, "/v1/login", "", body)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the third attempt should be 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("a 429 must carry Retry-After")
	}
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	router := newRateLimitedRouter(t, RateLimitPolicy{
		Limiter:       frozenLimiter(),
		LoginPerIP:    1,
		LoginPerEmail: 100,
	})

	first := do(t, router, http.MethodPost, "/v1/login", "", `{"email":"a@example.com","password":"x"}`)
	if first.Code == http.StatusTooManyRequests {
		t.Fatal("the first attempt should be allowed")
	}

	// A different address from the same IP is still the same caller.
	second := do(t, router, http.MethodPost, "/v1/login", "", `{"email":"b@example.com","password":"x"}`)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("a second address from one IP should be 429, got %d", second.Code)
	}
}

func TestSignupIsRateLimitedPerAddress(t *testing.T) {
	router := newRateLimitedRouter(t, RateLimitPolicy{
		Limiter:        frozenLimiter(),
		SignupPerIP:    100,
		SignupPerEmail: 1,
	})
	body := `{"email":"new@example.com","password":"pw","confirm_password":"pw"}`

	first := do(t, router, http.MethodPost, "/v1/signup", "", body)
	if first.Code == http.StatusTooManyRequests {
		t.Fatal("the first signup should be allowed")
	}
	second := do(t, router, http.MethodPost, "/v1/signup", "", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("the second signup for one address should be 429, got %d", second.Code)
	}
}

func TestTrustProxyUsesTheForwardedAddress(t *testing.T) {
	// Trusting the proxy header: two different real clients from the same
	// RemoteAddr (the proxy) each get their own counter.
	trusted := newRateLimitedRouter(t, RateLimitPolicy{
		Limiter:    frozenLimiter(),
		TrustProxy: true,
		LoginPerIP: 1,
	})
	if code := postLoginFrom(t, trusted, "203.0.113.1", "a@example.com"); code == http.StatusTooManyRequests {
		t.Fatal("the first forwarded client should be allowed")
	}
	if code := postLoginFrom(t, trusted, "203.0.113.2", "b@example.com"); code == http.StatusTooManyRequests {
		t.Fatal("a different forwarded client must have its own counter")
	}

	// Not trusting it: the header is ignored and the proxy's own address is the
	// key, so the second request is the same caller.
	untrusted := newRateLimitedRouter(t, RateLimitPolicy{
		Limiter:    frozenLimiter(),
		TrustProxy: false,
		LoginPerIP: 1,
	})
	if code := postLoginFrom(t, untrusted, "203.0.113.1", "a@example.com"); code == http.StatusTooManyRequests {
		t.Fatal("the first attempt should be allowed")
	}
	if code := postLoginFrom(t, untrusted, "203.0.113.2", "b@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("with TrustProxy off the header must be ignored, got %d", code)
	}
}

// Without a limiter nothing is throttled — the tests above would pass trivially
// if the handler returned 429 for another reason.
func TestDisabledLimiterDoesNotThrottle(t *testing.T) {
	router := newRateLimitedRouter(t, RateLimitPolicy{})
	body := `{"email":"a@example.com","password":"wrong"}`

	for i := 0; i < 10; i++ {
		if rec := do(t, router, http.MethodPost, "/v1/login", "", body); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was throttled; the limiter is disabled", i+1)
		}
	}
}

func postLoginFrom(t *testing.T, router *chi.Mux, realIP, email string) int {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/login", body)
	req.Header.Set("X-Real-IP", realIP)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder.Code
}
