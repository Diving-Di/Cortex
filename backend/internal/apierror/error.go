package apierror

import "net/http"

type Error struct {
	Code       string
	Message    string
	StatusCode int
	Details    any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, StatusCode: status}
}

func Validation(details any) *Error {
	return &Error{
		Code:       "VALIDATION_ERROR",
		Message:    "请求参数无效",
		StatusCode: http.StatusUnprocessableEntity,
		Details:    details,
	}
}
