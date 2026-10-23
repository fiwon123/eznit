package commands

import (
	"time"

	"github.com/google/uuid"
)

// CommandData is one command of the library.
//
// Content is set for a text command. A binary command carries ContentBase64
// instead, so a client can write it without a second request.
type CommandData struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	Size          int64     `json:"size"`
	Content       string    `json:"content,omitempty"`
	ContentBase64 string    `json:"content_base64,omitempty"`
	IsExecutable  bool      `json:"is_executable"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CommandRequest is the body that creates or replaces a command.
//
// A text command sends Content, a binary one sends ContentBase64 — the body is
// JSON, and arbitrary bytes do not survive it. IsExecutable is a pointer so
// "not sent" can mean the default (true) rather than false.
type CommandRequest struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Content       string `json:"content"`
	ContentBase64 string `json:"content_base64"`
	IsExecutable  *bool  `json:"is_executable"`
}

// ListResponse is the caller's library.
type ListResponse struct {
	Data  []CommandData `json:"data"`
	Total int           `json:"total"`
}
