package auth

import (
	"encoding/json"
	"strconv"
	"testing"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
