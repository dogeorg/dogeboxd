package system

import (
	_ "embed"
	"encoding/json"
)

type Timezone struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var (
	// File timezones.json contains manually generated timezone list.
	// TODO: Find a way to automatically generate this list
	//go:embed timezones.json
	tz_data        []byte
	tz_precompiled = func() (s []Timezone) {
		if err := json.Unmarshal(tz_data, &s); err != nil {
			panic(err)
		}
		return
	}()
)

func GetTimezones() ([]Timezone, error) {
	return tz_precompiled, nil
}
