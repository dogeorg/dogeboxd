package system

import (
	"fmt"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/go-resty/resty/v2"
)

func SubmitToReflector(config dogeboxd.ServerConfig, token, localIP string) error {
	client := resty.New()
	client.SetBaseURL(config.ReflectorHost)
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	resp, err := client.R().
		SetBody(map[string]string{"token": token, "ip": localIP}).
		Post("/")

	if err != nil {
		return err
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("failed to submit to reflector: %s", resp.String())
	}

	return nil
}
