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

func PrettyPrintDiskSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/float64(TB))
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
