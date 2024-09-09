package dogeboxd

import (
	"net/url"
	"strings"
)

type PupRouter struct {
	pm PupManager
}

// matchRoute checks if a URL matches any of the given route patterns, including wildcard segments.
func (t PupManager) matchRoute(urlStr string, routes []string) (matchedRoute string, ok bool) {
	// Parse the URL to get the path
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", false
	}

	// Extract and normalize the path from the URL
	path := strings.TrimRight(u.Path, "/")
	pathSegments := strings.Split(path, "/")

	for _, route := range routes {
		route = strings.Split(route, "/")
		routeSegments := strings.Split(route, "/")

		// Check if the route and path have the same number of segments or if the route has one less (due to a wildcard)
		if len(routeSegments) > len(pathSegments) || len(routeSegments) == len(pathSegments)-1 {
			continue
		}

		wildcardFound := false
		match := true

		for i, segment := range routeSegments {
			if segment == "*" {
				if wildcardFound {
					// More than one wildcard is not allowed
					match = false
					break
				}
				wildcardFound = true
			} else if i >= len(pathSegments) || segment != pathSegments[i] {
				match = false
				break
			}
		}

		if match {
			return route, true
		}
	}

	// No match found
	return "", false
}

// func main() {
// 	routes := []string{"/foo/*/bar", "/baz", "/qux/*"}
// 	urlsToTest := []string{
// 		"https://example.com/foo/test/bar",
// 		"https://example.com/baz",
// 		"https://example.com/qux/anything",
// 		"https://example.com/foo/test",
// 		"https://example.com/qux",
// 	}
//
// 	for _, urlStr := range urlsToTest {
// 		if route, ok := matchRoute(urlStr, routes); ok {
// 			println(urlStr, "matches", route)
// 		} else {
// 			println(urlStr, "does not match any route")
// 		}
// 	}
// }
