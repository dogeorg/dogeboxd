package dogeboxd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"

	"github.com/go-resty/resty/v2"
	"github.com/gorilla/securecookie"
)

type DKMManager interface {
	Authenticate(password string) (string, error)
	RefreshToken(old string) (string, bool, error)
	InvalidateToken(token string) (bool, error)
}

type DKMResponseLogin struct {
	AuthenticationToken string `json:"token"`
}

type DKMResponseRefreshToken struct {
	// TODO
	OK       bool   `json:"ok"`
	NewToken string `json:"token"`
}

type DKMResponseInvalidateToken struct {
	// TODO
	OK bool `json:"ok"`
}

type dkmManager struct {
	dkmPup PupManifest
	client *resty.Client
}

func NewDKMManager(dbx Dogeboxd) DKMManager {
	// TODO: get the dkm pup from our internal state
	dkmPup, found, err := PupManifest{}, true, error(nil)

	if err != nil {
		log.Fatalln("Failed to find an instance of DKM:", err)
	}

	if !found {
		// You can't use dogebox without an instance of DKM
		log.Fatalln("Could not find an instance of DKM installed. Aborting.")
	}

	client := resty.New()
	client.SetBaseURL(fmt.Sprintf("http://%s:80", dkmPup.containerIP))
	client.SetHeader("Accept", "application/json")
	client.SetContentLength(true)

	return dkmManager{
		dkmPup: dkmPup,
		client: client,
	}
}

func (t dkmManager) Authenticate(password string) (string, error) {
	// TODO: actually do this call once we have DKM setup properly.

	if password != "password1" {
		return "", errors.New("invalid password")
	}

	fakeDKMTokenBytes := securecookie.GenerateRandomKey(32)
	if fakeDKMTokenBytes == nil {
		return "", errors.New("Failed to generate token")
	}

	fakeDKMToken := make([]byte, hex.EncodedLen(len(fakeDKMTokenBytes)))
	hex.Encode(fakeDKMToken, fakeDKMTokenBytes)

	return string(fakeDKMToken), nil
	// var result DKMResponseLogin
	// _, err := t.client.R().SetBody(map[string]string{"password": password}).SetResult(&result).Post("/login")

	// if err != nil {
	// 	log.Println("Failed to contact DKM:", err)
	// 	return "", err
	// }

	// return result.AuthenticationToken, nil
}

func (t dkmManager) RefreshToken(oldToken string) (string, bool, error) {
	var result DKMResponseRefreshToken
	_, err := t.client.R().SetBody(map[string]string{"token": oldToken}).SetResult(&result).Post("/refresh-token")

	if err != nil {
		log.Println("Failed to contact DKM:", err)
		return "", false, err
	}

	if !result.OK {
		return "", false, nil
	}

	return result.NewToken, true, nil
}

func (t dkmManager) InvalidateToken(token string) (bool, error) {
	var result DKMResponseInvalidateToken
	_, err := t.client.R().SetBody(map[string]string{"token": token}).SetResult(&result).Post("/invalidate-token")

	if err != nil {
		log.Println("Failed to contact DKM:", err)
		return false, err
	}

	if !result.OK {
		return false, nil
	}

	return true, nil
}
