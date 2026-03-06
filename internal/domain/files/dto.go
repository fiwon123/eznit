package files

import "github.com/google/uuid"

type FileData struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Ext     string    `json:"ext"`
	Version int       `json:"version"`
}

type ListResponse struct {
	Data  []FileData `json:"data"`
	Total int        `json:"total"`
}
