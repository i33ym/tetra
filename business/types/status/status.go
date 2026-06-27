// Package status represents the lifecycle status of a payload as a validated
// value object.
package status

import "fmt"

// Set of known statuses.
var (
	Pending    = Status{"pending"}
	Processing = Status{"processing"}
	Done       = Status{"done"}
	Failed     = Status{"failed"}
)

var statuses = map[string]Status{
	Pending.value:    Pending,
	Processing.value: Processing,
	Done.value:       Done,
	Failed.value:     Failed,
}

// Status represents a payload status in the system.
type Status struct {
	value string
}

// Parse parses the string value and returns a status if one exists.
func Parse(value string) (Status, error) {
	s, exists := statuses[value]
	if !exists {
		return Status{}, fmt.Errorf("invalid status %q", value)
	}

	return s, nil
}

// MustParse parses the string value and returns a status if one exists. If an
// error occurs the function panics.
func MustParse(value string) Status {
	s, err := Parse(value)
	if err != nil {
		panic(err)
	}

	return s
}

// String returns the name of the status.
func (s Status) String() string {
	return s.value
}

// Equal provides support for the go-cmp package and testing.
func (s Status) Equal(s2 Status) bool {
	return s.value == s2.value
}

// MarshalText provides support for logging and any marshal needs.
func (s Status) MarshalText() ([]byte, error) {
	return []byte(s.value), nil
}
