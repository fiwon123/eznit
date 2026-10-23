package files

import "github.com/google/uuid"

// File Data
type FileData struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Ext      string    `json:"ext"`
	Version  int       `json:"version"`
	IsPublic bool      `json:"is_public"`
}

// SharedFileData is a shared file shown in the catalogue.
type SharedFileData struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Ext        string    `json:"ext"`
	Version    int       `json:"version"`
	IsPublic   bool      `json:"is_public"`
	Owner      string    `json:"owner"`
	OwnerEmail string    `json:"owner_email"`
}

// ShareRequest toggles sharing for a file.
type ShareRequest struct {
	IsPublic bool `json:"is_public"`
}

// File List Response
type ListResponse struct {
	Data  []FileData `json:"data"`
	Total int        `json:"total"`
}

// SharedListResponse lists shared files in the catalogue.
type SharedListResponse struct {
	Data  []SharedFileData `json:"data"`
	Total int              `json:"total"`
}
