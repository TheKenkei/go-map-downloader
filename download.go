package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

// Download file downloader
func Download(url, filePath string) error {

	// print(url)
	// print(url)
	// print(url)

	// print("\n")

	res, err := http.Get(url)

	/* 	print(res.Status)
	   	print("\n") */

	if err != nil {
		return fmt.Errorf("get file error: %+v", err)
	}

	if res.Status != "200 OK" {
		print(url)
		print("\n")
		return fmt.Errorf("get file error: %+v", err)

	}
	defer res.Body.Close()

	err = os.MkdirAll(path.Dir(filePath), os.FileMode(0777))
	if err != nil {
		return fmt.Errorf("create dir error: %+v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file error: %+v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, res.Body)
	if err != nil {
		return fmt.Errorf("save file error: %+v", err)
	}

	return nil
}
