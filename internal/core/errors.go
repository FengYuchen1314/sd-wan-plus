package core

import "fmt"

type ErrorCode string

const (
	ErrUnauthorized       ErrorCode = "UNAUTHORIZED"
	ErrForbidden          ErrorCode = "FORBIDDEN"
	ErrNotFound           ErrorCode = "NOT_FOUND"
	ErrConflict           ErrorCode = "CONFLICT"
	ErrValidation         ErrorCode = "VALIDATION"
	ErrRateLimited        ErrorCode = "RATE_LIMITED"
	ErrTokenExpired       ErrorCode = "TOKEN_EXPIRED"
	ErrTokenUsed          ErrorCode = "TOKEN_USED"
	ErrTokenRevoked       ErrorCode = "TOKEN_REVOKED"
	ErrNoLink             ErrorCode = "NO_LINK"
	ErrForwardingLoop     ErrorCode = "FORWARDING_LOOP"
	ErrPolicyConflict     ErrorCode = "POLICY_CONFLICT"
	ErrConfigFailed       ErrorCode = "CONFIG_FAILED"
	ErrNetdFailed         ErrorCode = "NETD_FAILED"
	ErrUpdateFailed       ErrorCode = "UPDATE_FAILED"
	ErrSignatureInvalid   ErrorCode = "SIGNATURE_INVALID"
	ErrInternal           ErrorCode = "INTERNAL"
)

type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Cause   error     `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Cause }

func NewError(code ErrorCode, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

func WrapError(code ErrorCode, msg string, cause error) *AppError {
	return &AppError{Code: code, Message: msg, Cause: cause}
}
