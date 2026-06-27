package errs

import (
	"net/http"
)

var (
	// None indicates the operation was successful.
	None = ErrCode{value: 0}

	// NoContent indicates the operation was successful with no content.
	NoContent = ErrCode{value: 1}

	// Canceled indicates the operation was canceled (typically by the caller).
	Canceled = ErrCode{value: 2}

	// Unknown error.
	Unknown = ErrCode{value: 3}

	// InvalidArgument indicates client specified an invalid argument.
	InvalidArgument = ErrCode{value: 4}

	// DeadlineExceeded means operation expired before completion.
	DeadlineExceeded = ErrCode{value: 5}

	// NotFound means some requested entity was not found.
	NotFound = ErrCode{value: 6}

	// AlreadyExists means an attempt to create an entity failed because one
	// already exists.
	AlreadyExists = ErrCode{value: 7}

	// PermissionDenied indicates the caller does not have permission.
	PermissionDenied = ErrCode{value: 8}

	// ResourceExhausted indicates some resource has been exhausted.
	ResourceExhausted = ErrCode{value: 9}

	// FailedPrecondition indicates operation was rejected because the system is
	// not in a state required for the operation's execution.
	FailedPrecondition = ErrCode{value: 10}

	// Aborted indicates the operation was aborted, typically due to a
	// concurrency issue.
	Aborted = ErrCode{value: 11}

	// OutOfRange means operation was attempted past the valid range.
	OutOfRange = ErrCode{value: 12}

	// Unimplemented indicates operation is not implemented.
	Unimplemented = ErrCode{value: 13}

	// Internal errors. Means some invariants expected by underlying system have
	// been broken.
	Internal = ErrCode{value: 14}

	// Unavailable indicates the service is currently unavailable.
	Unavailable = ErrCode{value: 15}

	// DataLoss indicates unrecoverable data loss or corruption.
	DataLoss = ErrCode{value: 16}

	// Unauthenticated indicates the request does not have valid authentication
	// credentials for the operation.
	Unauthenticated = ErrCode{value: 17}

	// TooManyRequests indicates the client has exceeded their rate limit.
	TooManyRequests = ErrCode{value: 18}

	// InternalOnlyLog errors. The error message is not sent to the client.
	InternalOnlyLog = ErrCode{value: 19}
)

var codeNumbers = map[string]ErrCode{
	"ok":                  None,
	"no_content":          NoContent,
	"canceled":            Canceled,
	"unknown":             Unknown,
	"invalid_argument":    InvalidArgument,
	"deadline_exceeded":   DeadlineExceeded,
	"not_found":           NotFound,
	"already_exists":      AlreadyExists,
	"permission_denied":   PermissionDenied,
	"resource_exhausted":  ResourceExhausted,
	"failed_precondition": FailedPrecondition,
	"aborted":             Aborted,
	"out_of_range":        OutOfRange,
	"unimplemented":       Unimplemented,
	"internal":            Internal,
	"unavailable":         Unavailable,
	"data_loss":           DataLoss,
	"unauthenticated":     Unauthenticated,
	"too_many_requests":   TooManyRequests,
	"internal_only_log":   InternalOnlyLog,
}

var codeNames = map[ErrCode]string{
	None:               "ok",
	NoContent:          "ok_no_content",
	Canceled:           "canceled",
	Unknown:            "unknown",
	InvalidArgument:    "invalid_argument",
	DeadlineExceeded:   "deadline_exceeded",
	NotFound:           "not_found",
	AlreadyExists:      "already_exists",
	PermissionDenied:   "permission_denied",
	ResourceExhausted:  "resource_exhausted",
	FailedPrecondition: "failed_precondition",
	Aborted:            "aborted",
	OutOfRange:         "out_of_range",
	Unimplemented:      "unimplemented",
	Internal:           "internal",
	Unavailable:        "unavailable",
	DataLoss:           "data_loss",
	Unauthenticated:    "unauthenticated",
	TooManyRequests:    "too_many_requests",
	InternalOnlyLog:    "internal_only_log",
}

var httpStatus = map[ErrCode]int{
	None:               http.StatusOK,
	NoContent:          http.StatusNoContent,
	Canceled:           http.StatusGatewayTimeout,
	Unknown:            http.StatusInternalServerError,
	InvalidArgument:    http.StatusBadRequest,
	DeadlineExceeded:   http.StatusGatewayTimeout,
	NotFound:           http.StatusNotFound,
	AlreadyExists:      http.StatusConflict,
	PermissionDenied:   http.StatusForbidden,
	ResourceExhausted:  http.StatusTooManyRequests,
	FailedPrecondition: http.StatusBadRequest,
	Aborted:            http.StatusConflict,
	OutOfRange:         http.StatusBadRequest,
	Unimplemented:      http.StatusNotImplemented,
	Internal:           http.StatusInternalServerError,
	Unavailable:        http.StatusServiceUnavailable,
	DataLoss:           http.StatusInternalServerError,
	Unauthenticated:    http.StatusUnauthorized,
	TooManyRequests:    http.StatusTooManyRequests,
	InternalOnlyLog:    http.StatusInternalServerError,
}
