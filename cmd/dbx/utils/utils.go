package utils

import "os"

func ExitBad(isSystemd bool) {
	if isSystemd {
		os.Exit(255)
		return
	}

	os.Exit(1)
}
