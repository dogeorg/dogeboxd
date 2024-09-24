package utils

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
)

func ImageBytesToWebBase64(imgBytes []byte, filename string) (string, error) {
	logoData64 := base64.StdEncoding.EncodeToString(imgBytes)
	contentType := ""

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}

	return "data:" + contentType + ";base64," + logoData64, nil
}
