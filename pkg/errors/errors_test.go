package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewAppError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
	}{
		{"not found", 404, "file not found"},
		{"unauthorized", 401, "invalid credentials"},
		{"server error", 500, "could not read the account"},
		{"empty message", 400, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewAppError(tt.statusCode, tt.message)

			if got := err.Error(); got != tt.message {
				t.Errorf("Error() = %q, want %q", got, tt.message)
			}
			if got := err.StatusCode(); got != tt.statusCode {
				t.Errorf("StatusCode() = %d, want %d", got, tt.statusCode)
			}
		})
	}
}

// A handler writes the status the service chose, so the type must survive the
// wrapping a layer may add around it: errors.As (not a type assertion) is what
// the callers use.
func TestAppErrorThroughWrapping(t *testing.T) {
	appErr := NewAppError(409, "command in use")

	wrapped := fmt.Errorf("delete command: %w", appErr)

	var got *AppError
	if !errors.As(wrapped, &got) {
		t.Fatalf("errors.As(%v, *AppError) = false, want true", wrapped)
	}
	if got.StatusCode() != 409 {
		t.Errorf("StatusCode() = %d, want 409", got.StatusCode())
	}
	if got.Error() != "command in use" {
		t.Errorf("Error() = %q, want %q", got.Error(), "command in use")
	}
}

// Two errors built with the same values are distinct values: the type is a
// message carrier, not a sentinel, so errors.Is must not treat one as the
// other. A future change to the sentinel pattern would have to change this
// test on purpose.
func TestAppErrorIsNotASentinel(t *testing.T) {
	first := NewAppError(404, "not found")
	second := NewAppError(404, "not found")

	if errors.Is(first, second) {
		t.Error("errors.Is(first, second) = true, want false: the type is not a sentinel")
	}
}
