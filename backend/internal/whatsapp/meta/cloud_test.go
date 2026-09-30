package meta

import (
	"strings"
	"testing"
)

func TestRetryableAndWords(t *testing.T) {
	if !Retryable(&APIError{Status: 500}) || !Retryable(&APIError{Status: 400, Code: 130429}) {
		t.Fatal("server errors and rate limits are retried")
	}
	if Retryable(&APIError{Status: 400, Code: 131047}) {
		t.Fatal("a closed window is final")
	}
	if !strings.Contains(Describe(131047, ""), "24 saat") {
		t.Fatal("the window error must say 24 hours")
	}
}
