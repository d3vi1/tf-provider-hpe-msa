package msa

import "strings"

// CommandPath converts CLI-style commands into the XML API path.
// Example: CommandPath("show", "pools") => "/api/show/pools".
func CommandPath(parts ...string) string {
	segments := []string{"api"}
	for _, part := range parts {
		for _, token := range strings.Fields(part) {
			if token != "" {
				segments = append(segments, token)
			}
		}
	}
	return "/" + strings.Join(segments, "/")
}
