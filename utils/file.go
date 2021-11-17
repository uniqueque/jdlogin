package utils

import (
	"encoding/base64"
	"os"
)

//保存base64数据为文件
func SaveBase64ToFile(content, path string) error {
	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	f.Write(data)
	return nil
}
