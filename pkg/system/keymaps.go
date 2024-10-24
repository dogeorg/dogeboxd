package system

import (
	"bufio"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

type Keymap struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func GetKeymaps() ([]Keymap, error) {
	// Execute the xkbcli command
	cmd := exec.Command("xkbcli", "list", "--load-exotic")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Convert output to a string and create a scanner
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	// Regular expressions to match layout and description
	layoutRegex := regexp.MustCompile(`layout: *'([^']+)'`)
	descriptionRegex := regexp.MustCompile(`description: *(.*)`)

	var layouts []Keymap
	layoutMap := make(map[string]bool)
	var currentLayout string

	for scanner.Scan() {
		line := scanner.Text()

		// Match the layout line
		if layoutMatch := layoutRegex.FindStringSubmatch(line); layoutMatch != nil {
			layout := layoutMatch[1]
			if layoutMap[layout] {
				continue
			}
			currentLayout = layout
			layoutMap[layout] = true
		}

		// Match the description line and add to the slice
		if currentLayout != "" {
			if descMatch := descriptionRegex.FindStringSubmatch(line); descMatch != nil {
				description := descMatch[1]
				layouts = append(layouts, Keymap{Name: currentLayout, Value: description})
				currentLayout = ""
			}
		}
	}

	// Sort layouts by name for consistent output
	sort.Slice(layouts, func(i, j int) bool {
		return layouts[i].Name < layouts[j].Name
	})

	// Remove layouts with name "custom"
	filteredLayouts := make([]Keymap, 0, len(layouts))
	for _, layout := range layouts {
		if layout.Name != "custom" {
			filteredLayouts = append(filteredLayouts, layout)
		}
	}

	return filteredLayouts, nil
}
