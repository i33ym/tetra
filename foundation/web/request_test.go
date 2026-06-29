package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sample struct {
	Name string `json:"name"`
}

func newReq(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
}

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string // substring; "" means no error expected
	}{
		{"valid", `{"name":"x"}`, ""},
		{"empty body", ``, "must not be empty"},
		{"unknown field", `{"name":"x","nope":1}`, "unknown key"},
		{"badly formed", `{"name":}`, "badly-formed JSON"},
		{"wrong type", `{"name":123}`, "incorrect JSON type"},
		{"trailing data", `{"name":"x"}{}`, "single JSON value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst sample
			err := DecodeJSON(newReq(tt.body), 1<<20, &dst)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got error %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeJSONMaxBytes(t *testing.T) {
	body := `{"name":"` + strings.Repeat("a", 200) + `"}`

	var dst sample
	err := DecodeJSON(newReq(body), 16, &dst)

	if err == nil || !strings.Contains(err.Error(), "must not be larger") {
		t.Fatalf("got error %v, want a max-bytes error", err)
	}
}
