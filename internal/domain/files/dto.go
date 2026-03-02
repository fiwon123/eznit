package files

type FileData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Ext     string `json:"ext"`
	Version int    `json:"version"`
}

type ListResponse struct {
	Data  []FileData `json:"data"`
	Total int        `json:"total"`
}
