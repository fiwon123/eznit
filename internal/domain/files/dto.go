package files

type StorageRequest struct {
}

type DataResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Ext  string `json:"ext"`
}

type MsgResponse struct {
	Msg string `json:"message"`
}
