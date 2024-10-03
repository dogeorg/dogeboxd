package system

import (
	"fmt"
	"log"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/go-resty/resty/v2"
)

func SubmitToReflector(config dogeboxd.ServerConfig, host, token, localIP string) error {
	if config.DisableReflector {
		log.Println("Reflector disabled, skipping submission")
		return nil
	}

	client := resty.New()
	client.SetBaseURL(host)
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	resp, err := client.R().
		SetBody(map[string]string{"token": token, "ip": localIP}).
		Post("/")

	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to submit to reflector: %s", resp.String())
	}

	return nil
}
