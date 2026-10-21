package fortivpn

import (
	"errors"
	"fmt"
)

// Code identifies a structured error category returned by the Fortinet SSL-VPN engine.
type Code string

const (
	CodeInvalidConfig       Code = "invalid_config"
	CodeGatewayUnreachable  Code = "gateway_unreachable"
	CodeCertUntrusted       Code = "cert_untrusted"
	CodeAuthFailed          Code = "auth_failed"
	CodeOTPRequired         Code = "otp_required"
	CodeOTPInvalid          Code = "otp_invalid"
	CodePromptTimeout       Code = "prompt_timeout"
	CodeTunnelDenied        Code = "tunnel_denied"
	CodePPPFailed           Code = "ppp_failed"
	CodePeerDead            Code = "peer_dead"
	CodeConnectionLost      Code = "connection_lost"
	CodeDeviceFailed        Code = "device_failed"
	CodeNetcfgFailed        Code = "netcfg_failed"
	CodeUnsupportedPlatform Code = "unsupported_platform"
)

// Error represents a structured error from the Fortinet SSL-VPN engine.
// Detail is always English and sanitized of secrets.
type Error struct {
	Code        Code   `json:"code"`
	Detail      string `json:"detail,omitempty"`
	Field       string `json:"field,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Cause       error  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Field != "" && e.Detail != "" {
		return fmt.Sprintf("fortivpn [%s:%s]: %s", e.Code, e.Field, e.Detail)
	}
	if e.Detail != "" {
		return fmt.Sprintf("fortivpn [%s]: %s", e.Code, e.Detail)
	}
	if e.Cause != nil {
		return fmt.Sprintf("fortivpn [%s]: %v", e.Code, e.Cause)
	}
	return fmt.Sprintf("fortivpn [%s]", e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Retryable reports whether the error is a transient transport or negotiation
// failure eligible for automatic reconnection.
func (e *Error) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Code {
	case CodeGatewayUnreachable, CodePPPFailed, CodePeerDead, CodeConnectionLost:
		return true
	default:
		return false
	}
}

// NewError creates a structured *Error with the given code and detail message.
func NewError(code Code, detail string) *Error {
	return &Error{Code: code, Detail: detail}
}

// WrapError wraps a cause into a structured *Error.
func WrapError(code Code, detail string, cause error) *Error {
	if detail == "" && cause != nil {
		detail = cause.Error()
	}
	return &Error{Code: code, Detail: detail, Cause: cause}
}

// NewFieldError creates an invalid_config *Error for a specific field.
func NewFieldError(field, detail string) *Error {
	return &Error{
		Code:   CodeInvalidConfig,
		Field:  field,
		Detail: detail,
	}
}

// AsError extracts a *Error from err if present.
func AsError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// IsCode reports whether err wraps an *Error with the specified Code.
func IsCode(err error, code Code) bool {
	fe, ok := AsError(err)
	return ok && fe.Code == code
}
