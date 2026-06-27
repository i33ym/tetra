package web

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Param returns the URL path parameter for the given key.
func Param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// Decoder represents data that can be decoded.
type Decoder interface {
	Decode(data []byte) error
}

type validator interface {
	Validate() error
}

// Decode reads the body of an HTTP request and decodes the body into the
// specified data model. If the data model implements the validator interface,
// the method will be called.
func Decode(r *http.Request, v Decoder) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("request: unable to read payload: %w", err)
	}

	if err := v.Decode(data); err != nil {
		return fmt.Errorf("request: decode: %w", err)
	}

	if v, ok := v.(validator); ok {
		if err := v.Validate(); err != nil {
			return err
		}
	}

	return nil
}
