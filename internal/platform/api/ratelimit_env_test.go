package api

import (
	"testing"
	"time"
)

var rateLimitVars = []string{
	"RATE_LIMIT_ENABLED",
	"TRUST_PROXY",
	"RATE_LIMIT_WINDOW",
	"RATE_LIMIT_LOGIN_IP",
	"RATE_LIMIT_LOGIN_EMAIL",
	"RATE_LIMIT_SIGNUP_IP",
	"RATE_LIMIT_SIGNUP_EMAIL",
}

func TestRateLimitFromEnvDefaults(t *testing.T) {
	for _, k := range rateLimitVars {
		t.Setenv(k, "")
	}

	c := RateLimitFromEnv()
	if !c.Enabled {
		t.Error("rate limiting should be on by default")
	}
	if c.TrustProxy {
		t.Error("TRUST_PROXY should be off by default")
	}
	if c.Window != 15*time.Minute {
		t.Errorf("Window = %s, want 15m", c.Window)
	}
	if c.LoginPerIP != 20 || c.LoginPerEmail != 10 || c.SignupPerIP != 10 || c.SignupPerEmail != 5 {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestRateLimitFromEnvAcceptsQuotedValues(t *testing.T) {
	for _, k := range rateLimitVars {
		t.Setenv(k, "")
	}
	// `.env` may carry quoted values; they must not break the parse.
	t.Setenv("RATE_LIMIT_ENABLED", `"false"`)
	t.Setenv("TRUST_PROXY", `"true"`)
	t.Setenv("RATE_LIMIT_WINDOW", `"30m"`)
	t.Setenv("RATE_LIMIT_LOGIN_IP", `"5"`)

	c := RateLimitFromEnv()
	if c.Enabled {
		t.Error("RATE_LIMIT_ENABLED=\"false\" should turn it off")
	}
	if !c.TrustProxy {
		t.Error("TRUST_PROXY=\"true\" should turn it on")
	}
	if c.Window != 30*time.Minute {
		t.Errorf("Window = %s, want 30m", c.Window)
	}
	if c.LoginPerIP != 5 {
		t.Errorf("LoginPerIP = %d, want 5", c.LoginPerIP)
	}
	// Unset values keep their defaults.
	if c.LoginPerEmail != 10 {
		t.Errorf("LoginPerEmail = %d, want the default 10", c.LoginPerEmail)
	}
}
