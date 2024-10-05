package system

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/go-resty/resty/v2"
)

type ReflectorFileData struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

func SaveReflectorTokenForReboot(config dogeboxd.ServerConfig, host, token string) error {
	data := ReflectorFileData{
		Host:  host,
		Token: token,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal reflector data: %w", err)
	}

	filePath := filepath.Join(config.DataDir, "reflector.json")
	err = os.WriteFile(filePath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write reflector data: %w", err)
	}

	return nil
}

func CheckAndSubmitReflectorData(config dogeboxd.ServerConfig, localIP string) error {
	if config.DisableReflector {
		log.Println("Reflector disabled, skipping checking")
		return nil
	}

	filePath := filepath.Join(config.DataDir, "reflector.json")
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read reflector data file: %w", err)
	}

	var data ReflectorFileData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		log.Println("invalid reflector data: host or token is empty")
		return nil
	}

	host := data.Host
	token := data.Token

	if host == "" || token == "" {
		log.Println("invalid reflector data: host or token is empty")
		return nil
	}

	log.Printf("Submitting reflector data to %s w/ token %s", host, token)

	client := resty.New()
	client.SetBaseURL(host)
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	resp, err := client.R().
		SetBody(map[string]string{"token": token, "ip": localIP}).
		Post("/")

	if err != nil {
		log.Printf("Failed to submit to reflector: %s", err)
		return err
	}

	if resp.StatusCode() != http.StatusCreated {
		log.Printf("Failed to submit to reflector: %s", resp.String())
		return fmt.Errorf("failed to submit to reflector: %s", resp.String())
	}

	return nil
}
