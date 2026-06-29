package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Param returns the URL path parameter for the given key.
func Param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// validator is implemented by request models that can validate themselves
// after decoding.
type validator interface {
	Validate() error
}

// DecodeJSON reads a single JSON value from the request body into dst with
// strict, client-friendly error handling:
//
//   - the body is capped at maxBytes (when > 0) to bound memory,
//   - unknown fields are rejected (DisallowUnknownFields),
//   - the body must contain exactly one JSON value,
//   - malformed input yields a clear, safe message instead of a raw json error.
//
// If dst implements validator, its Validate method runs after a successful
// decode. The returned errors are safe to surface to the client (the caller
// typically wraps them as an InvalidArgument).
func DecodeJSON(r *http.Request, maxBytes int64, dst any) error {
	if maxBytes > 0 {
		r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return jsonDecodeError(err)
	}

	// A second decode must hit EOF, proving the body held a single JSON value.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain only a single JSON value")
	}

	if v, ok := dst.(validator); ok {
		if err := v.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func jsonDecodeError(err error) error {
	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError
	var invalidUnmarshalError *json.InvalidUnmarshalError
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.As(err, &syntaxError):
		return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

	case errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("body contains badly-formed JSON")

	case errors.As(err, &unmarshalTypeError):
		if unmarshalTypeError.Field != "" {
			return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
		}
		return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

	case errors.Is(err, io.EOF):
		return errors.New("body must not be empty")

	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return fmt.Errorf("body contains unknown key %s", fieldName)

	case errors.As(err, &maxBytesError):
		return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)

	case errors.As(err, &invalidUnmarshalError):
		// A nil or non-pointer dst is a programmer error, not bad input.
		panic(err)

	default:
		return err
	}
}
