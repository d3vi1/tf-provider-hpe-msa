//go:build acc

package acceptance

import (
	"os"
	"testing"
)

func TestAccPrereqs(t *testing.T) {
	required := []string{
		"MSA_ENDPOINT",
		"MSA_USERNAME",
		"MSA_PASSWORD",
		"MSA_INSECURE_TLS",
		"MSA_POOL",
	}

	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Skip("acceptance tests skipped; missing required environment variables")
		}
	}
}
