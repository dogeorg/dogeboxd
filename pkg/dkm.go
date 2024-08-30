package dogeboxd

import (
	"errors"
	"fmt"
	"log"

	"github.com/go-resty/resty/v2"
)

type DKMManager interface {
	CreateKey(password string) ([]string, error)
	// Returns "" as a token if the password supplied is invalid.
	Authenticate(password string) (string, error, error)
	RefreshToken(old string) (string, bool, error)
	InvalidateToken(token string) (bool, error)
}

type DKMResponseCreateKey struct {
	SeedPhrase []string `json:"seedphrase"`
}

type DKMResponseAuthenticate struct {
	AuthenticationToken string `json:"token"`
	ValidFor            int    `json:"valid_for"`
}

type DKMErrorResponse struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

type DKMResponseInvalidateToken struct{}

type dkmManager struct {
	client *resty.Client
}

func NewDKMManager(pupManager PupManager) DKMManager {
	// TODO: get the dkm pup from our internal state
	dkmIP := "127.0.0.1"

	// if err != nil {
	// 	log.Fatalln("Failed to find an instance of DKM:", err)
	// }

	// if !found {
	// 	// You can't use dogebox without an instance of DKM
	// 	log.Fatalln("Could not find an instance of DKM installed. Aborting.")
	// }

	client := resty.New()
	client.SetBaseURL(fmt.Sprintf("http://%s:8089", dkmIP))
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	return dkmManager{
		client: client,
	}
}

func (t dkmManager) CreateKey(password string) ([]string, error) {
	// TODO: we probably want to add some restrictions on passwords that can be used here?

	var result DKMResponseCreateKey
	var errorResponse DKMErrorResponse

	_, err := t.client.R().SetBody(map[string]string{
		"password": password,
	}).SetResult(&result).SetError(&errorResponse).Post("/create")

	if err != nil {
		log.Printf("Error calling DKM %+v", err)
		return []string{}, nil
	}

	if errorResponse.Error != "" {
		log.Printf("Error from DKM: [%s] %s", errorResponse.Error, errorResponse.Reason)
		return []string{}, errors.New(errorResponse.Reason)
	}

	return result.SeedPhrase, nil
}

func (t dkmManager) Authenticate(password string) (string, error, error) {
	var result DKMResponseAuthenticate
	var errorResponse DKMErrorResponse

	_, err := t.client.R().SetBody(map[string]string{"password": password}).SetResult(&result).SetError(&errorResponse).Post("/login")

	if err != nil {
		log.Println("Failed to contact DKM:", err)
		return "", nil, err
	}

	if errorResponse.Error != "" {
		log.Printf("Error from DKM: [%s] %s", errorResponse.Error, errorResponse.Reason)
		return "", errors.New(errorResponse.Reason), nil
	}

	return result.AuthenticationToken, nil, nil
}

func (t dkmManager) RefreshToken(oldToken string) (string, bool, error) {
	var result DKMResponseAuthenticate
	var errorResponse DKMErrorResponse

	_, err := t.client.R().SetBody(map[string]string{"token": oldToken}).SetResult(&result).SetError(&errorResponse).Post("/roll-token")

	if err != nil {
		log.Println("Failed to contact DKM:", err)
		return "", false, err
	}

	if errorResponse.Error != "" {
		log.Printf("Error from DKM: [%s] %s", errorResponse.Error, errorResponse.Reason)
		return "", false, errors.New(errorResponse.Reason)
	}

	return result.AuthenticationToken, true, nil
}

func (t dkmManager) InvalidateToken(token string) (bool, error) {
	var result DKMResponseInvalidateToken
	var errorResponse DKMErrorResponse

	_, err := t.client.R().SetBody(map[string]string{"token": token}).SetResult(&result).SetError(&errorResponse).Post("/logout")

	if err != nil {
		log.Println("Failed to contact DKM:", err)
		return false, err
	}

	if errorResponse.Error != "" {
		log.Printf("Error from DKM: [%s] %s", errorResponse.Error, errorResponse.Reason)
		return false, errors.New(errorResponse.Reason)
	}

	return true, nil
}
