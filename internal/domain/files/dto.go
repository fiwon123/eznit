package files

type FileData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Ext     string `json:"ext"`
	Version int    `json:"version"`
}

type SingleReponse struct {
	Data FileData `json:"data"`
}

type ListResponse struct {
	Data  []FileData `json:"data"`
	Total int        `json:"total"`
}

type MsgResponse struct {
	Msg string `json:"message"`
}
