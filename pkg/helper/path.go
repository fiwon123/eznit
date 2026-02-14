package helper

import (
	"fmt"
	"os"
)

func CreatePathIfNotExists(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}
