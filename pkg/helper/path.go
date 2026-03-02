package helper

import (
	"errors"
	"io/fs"
	"os"
)

func CreatePathIfNotExists(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	return nil
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
