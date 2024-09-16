package cmd

func IsAlphanumeric(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func IsAbsolutePath(path string) bool {
	return len(path) > 0 && path[0] == '/'
}
