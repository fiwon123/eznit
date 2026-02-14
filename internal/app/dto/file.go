package dto

type FileStorage struct {
}

type FileDataResponse struct {
	Name string `json:"name"`
	Ext  string `json:"ext"`
}

type FileMsgResponse struct {
	Msg string `json:"message"`
}
