package model

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

type Failure struct {
	Reason     string
	Retryable  bool
	HTTPStatus int
	RetryAfter *time.Duration
}

// Rules are ordered from specific terminal failures to transient transport
// failures. New cases belong here; budgets, UI and callers do not classify errors.
var failureRules = []func(error) (Failure, bool){
	cancelledFailure,
	certificateFailure,
	providerBusinessFailure,
	httpFailure,
	toolCallFailure,
	streamFailure,
	networkFailure,
}

func toolCallFailure(err error) (Failure, bool) {
	var invalid *ToolCallError
	return Failure{Reason: "invalid_tool_call", Retryable: true}, errors.As(err, &invalid)
}

func ClassifyFailure(err error) Failure {
	for _, rule := range failureRules {
		if failure, matched := rule(err); matched {
			var provider *ProviderError
			if errors.As(err, &provider) {
				failure.HTTPStatus = provider.HTTPStatus
				failure.RetryAfter = provider.retryAfter
			}
			return failure
		}
	}
	return Failure{Reason: "unknown"}
}

func cancelledFailure(err error) (Failure, bool) {
	return Failure{Reason: "cancelled"}, errors.Is(err, context.Canceled)
}

func certificateFailure(err error) (Failure, bool) {
	var invalid x509.CertificateInvalidError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var record tls.RecordHeaderError
	return Failure{Reason: "tls"}, errors.As(err, &invalid) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &record)
}

func providerBusinessFailure(err error) (Failure, bool) {
	var provider *ProviderError
	if !errors.As(err, &provider) {
		return Failure{}, false
	}
	var body struct {
		Code  json.RawMessage `json:"code"`
		Type  string          `json:"type"`
		Error struct {
			Code json.RawMessage `json:"code"`
			Type string          `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(provider.detail), &body) != nil {
		return Failure{}, false
	}
	code, kind := body.Code, body.Type
	if len(body.Error.Code) > 0 {
		code = body.Error.Code
	}
	if body.Error.Type != "" {
		kind = body.Error.Type
	}
	for _, value := range []string{strings.Trim(string(code), `"`), kind} {
		switch strings.ToLower(value) {
		case "invalid_api_key", "authentication_error", "permission_error":
			return Failure{Reason: "authentication"}, true
		case "insufficient_quota", "credit_balance_exhausted":
			return Failure{Reason: "quota"}, true
		case "context_length_exceeded", "invalid_request_error":
			return Failure{Reason: "invalid_request"}, true
		case "rate_limit_error", "rate_limit_exceeded":
			return Failure{Reason: "rate_limited", Retryable: true}, true
		case "overloaded_error", "server_error", "api_error":
			return Failure{Reason: "server", Retryable: true}, true
		}
	}
	return Failure{}, false
}

func httpFailure(err error) (Failure, bool) {
	var provider *ProviderError
	if !errors.As(err, &provider) || provider.HTTPStatus < 300 {
		return Failure{}, false
	}
	switch {
	case provider.HTTPStatus == 401 || provider.HTTPStatus == 403:
		return Failure{Reason: "authentication"}, true
	case provider.HTTPStatus == 408:
		return Failure{Reason: "timeout", Retryable: true}, true
	case provider.HTTPStatus == 429:
		return Failure{Reason: "rate_limited", Retryable: true}, true
	case provider.HTTPStatus >= 500:
		return Failure{Reason: "server", Retryable: true}, true
	default:
		return Failure{Reason: "invalid_request"}, true
	}
}

func streamFailure(err error) (Failure, bool) {
	return Failure{Reason: "stream_interrupted", Retryable: true},
		errors.Is(err, ErrStreamInterrupted) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

func networkFailure(err error) (Failure, bool) {
	var request *url.Error
	var network net.Error
	var read *streamReadError
	if errors.As(err, &network) && network.Timeout() {
		return Failure{Reason: "timeout", Retryable: true}, true
	}
	if errors.As(err, &request) || errors.As(err, &read) {
		// Only outbound transport errors are wrapped this way by the gateway.
		return Failure{Reason: "network", Retryable: true}, true
	}
	var provider *ProviderError
	if errors.As(err, &provider) && provider.cause != nil {
		// HTTP/2 stream resets while reading a response do not necessarily
		// implement net.Error. Keep the rule at the transport boundary.
		return Failure{Reason: "network", Retryable: true}, true
	}
	return Failure{}, false
}
