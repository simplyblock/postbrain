package providers

import "strings"

const defaultOllamaServiceURL = "http://localhost:11434"

func serviceURLOrDefault(url, fallback string) string {
	if u := strings.TrimSpace(url); u != "" {
		return u
	}
	return fallback
}
