package sql

import (
	"fmt"
	"os"
	"strings"
)

// DSNFromEnv builds the PostgreSQL connection string from the DB_* variables
// and the password. The API and the GUI both read it, so a database that one
// can reach is a database the other can reach.
func DSNFromEnv() string {
	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		PasswordFromEnv(),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
}

// PasswordFromEnv returns the database password. DB_PASSWORD_FILE — a path to
// a file holding the password — wins over DB_PWD, which exists for a
// throwaway local database. In the local stack the password is a Docker
// secret, so it is never an environment variable.
func PasswordFromEnv() string {
	if path := os.Getenv("DB_PASSWORD_FILE"); path != "" {
		if content, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	return os.Getenv("DB_PWD")
}
