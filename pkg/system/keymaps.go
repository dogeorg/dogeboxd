package system

import (
    _ "embed"
    "encoding/json"
)

type Keymap struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var (
    // File keymaps.json contains manually generated keymaps list.
    // TODO: Find a way to automatically generate this list
    //go:embed keymaps.json
    data []byte
    precompiled = func() (s []Keymap) {
        if err := json.Unmarshal(data, &s); err != nil {
            panic(err)
        }
        return
    }()
)


func GetKeymaps() ([]Keymap, error) {
	return precompiled, nil
}
