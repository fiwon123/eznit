package files

import (
	"mime/multipart"
	"net/http"

	"github.com/fiwon123/eznit/pkg/errors"
)

func extractFileID(r *http.Request) string {
	return r.PathValue("id")
}

func extractFile(r *http.Request) (multipart.File, *multipart.FileHeader, string, *errors.AppError) {

	// Parse the multipart form. 8MB stays in RAM, the rest goes to temp files.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return nil, nil, "", errors.NewAppError(http.StatusBadRequest, "file too big")
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, nil, "", errors.NewAppError(http.StatusBadRequest, "could not find file")
	}

	contentType := header.Header.Get("Content-Type")

	return file, header, contentType, nil
}
