package msa

import (
	"errors"
	"fmt"
	"strings"
)

type APIError struct {
	Status Status
}

func (e APIError) Error() string {
	response := strings.TrimSpace(e.Status.Response)
	if response == "" {
		return "command failed"
	}
	return fmt.Sprintf("command failed: %s", response)
}

func IsSessionError(err error) bool {
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	msg := strings.ToLower(apiErr.Status.Response)
	return strings.Contains(msg, "session") || strings.Contains(msg, "login") || strings.Contains(msg, "authorization")
}
