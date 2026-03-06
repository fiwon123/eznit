package files

import "github.com/oklog/ulid/v2"

type FileData struct {
	ID      ulid.ULID `json:"id"`
	Name    string    `json:"name"`
	Ext     string    `json:"ext"`
	Version int       `json:"version"`
}

type ListResponse struct {
	Data  []FileData `json:"data"`
	Total int        `json:"total"`
}
