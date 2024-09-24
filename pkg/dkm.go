package dogeboxd

import (
	"errors"
	"log"

	"github.com/go-resty/resty/v2"
)

type DKMManager interface {
	CreateKey(password string) ([]string, error)
	// Returns "" as a token if the password supplied is invalid.
	Authenticate(password string) (string, error, error)
	RefreshToken(old string) (string, bool, error)
	InvalidateToken(token string) (bool, error)
	MakeDelegate(id string, token string) (DKMResponseMakeDelegate, error)
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

type DKMRequestMakeDelegate struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type DKMResponseMakeDelegate struct {
	Pub  string `json:"pub"`
	Priv string `json:"priv"`
	Wif  string `json:"wif"`
}

type DKMErrorResponseMakeDelegate struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

type DKMResponseInvalidateToken struct{}

type dkmManager struct {
	client *resty.Client
}

func NewDKMManager() DKMManager {
	client := resty.New()
	client.SetBaseURL("http://127.0.0.1:8089")
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	return dkmManager{
		client: client,
	}
}

func (t dkmManager) CreateKey(password string) ([]string, error) {
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

func (t dkmManager) MakeDelegate(id string, token string) (DKMResponseMakeDelegate, error) {
	var result DKMResponseMakeDelegate
	var errorResponse DKMErrorResponseMakeDelegate

	body := DKMRequestMakeDelegate{
		ID:    id,
		Token: token,
	}

	_, err := t.client.R().SetBody(body).SetResult(&result).SetError(&errorResponse).Post("/make-delegate")
	if err != nil {
		log.Printf("Failed to contact DKM making delegate: %v", err)
		return result, err
	}

	if errorResponse.Error != "" {
		log.Printf("Error from DKM MakeDelegate: [%s] %s", errorResponse.Error, errorResponse.Reason)
		return result, errors.New(errorResponse.Reason)
	}

	return result, nil
}
