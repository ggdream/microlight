package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ggdream/microlight/internal/providerdiagnostic"
)

// Failure describes the search failure without exposing raw responses or credentials.
type Failure struct {
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
	Code       string `json:"code,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable"`
}

func (f *Failure) Error() string { return f.Message }

func DescribeFailure(err error) Failure {
	if failure, ok := errors.AsType[*Failure](err); ok {
		return *failure
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Failure{Kind: "timeout", Message: "The web search request timed out.", Retryable: true}
	}
	if network, ok := errors.AsType[net.Error](err); ok {
		if network.Timeout() {
			return Failure{Kind: "timeout", Message: "The web search request timed out.", Retryable: true}
		}
		return Failure{Kind: "network", Message: "Could not communicate with the search service.", Retryable: true}
	}
	return Failure{Kind: "invalid_response", Message: "The search service returned an unreadable or incomplete response."}
}

type upstreamError struct {
	code    string
	message string
}

type errorObject struct {
	Code    json.RawMessage `json:"code"`
	CodeN   json.RawMessage `json:"codeN"`
	Status  string          `json:"status"`
	Type    string          `json:"type"`
	Reason  string          `json:"reason"`
	Message string          `json:"message"`
	Info    string          `json:"info"`
	Msg     string          `json:"msg"`
	Text    string          `json:"text"`
	Error   string          `json:"error"`
	Errors  []errorObject   `json:"errors"`
}

func (e errorObject) diagnostic() upstreamError {
	code := first(codeText(e.Code), codeText(e.CodeN), e.Reason, e.Type, e.Status)
	message := first(e.Message, e.Info, e.Msg, e.Text, e.Error)
	if len(e.Errors) > 0 {
		nested := e.Errors[0].diagnostic()
		// Google uses a numeric status in code and a more specific reason in errors.
		code = first(nested.code, code)
		message = first(message, nested.message)
	}
	return upstreamError{code, message}
}

func parseSearchError(body []byte, httpFailed bool, id ID) (upstreamError, bool) {
	var envelope struct {
		errorObject
		Error        json.RawMessage `json:"error"`
		ErrorCode    json.RawMessage `json:"error_code"`
		ErrorMessage string          `json:"error_msg"`
		Detail       json.RawMessage `json:"detail"`
		Success      *bool           `json:"success"`
		Response     struct {
			Error *errorObject
		}
		ResponseMetadata struct {
			Error *errorObject
		}
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		// Some gateways add fields whose types differ from the API's error schema.
		// Keep the fields that decoded correctly when the response is valid JSON.
		var fieldErr *json.UnmarshalTypeError
		if !errors.As(err, &fieldErr) {
			return upstreamError{}, httpFailed
		}
	}
	result := envelope.errorObject.diagnostic()
	switch {
	case envelope.Response.Error != nil:
		return envelope.Response.Error.diagnostic(), true
	case envelope.ResponseMetadata.Error != nil:
		return envelope.ResponseMetadata.Error.diagnostic(), true
	case len(envelope.Error) > 0 && string(envelope.Error) != "null" && string(envelope.Error) != "false":
		var object errorObject
		if json.Unmarshal(envelope.Error, &object) == nil {
			nested := object.diagnostic()
			return upstreamError{first(nested.code, result.code), first(nested.message, result.message)}, true
		}
		var message string
		_ = json.Unmarshal(envelope.Error, &message)
		return upstreamError{result.code, first(message, result.message)}, true
	case len(envelope.ErrorCode) > 0:
		result.code = first(codeText(envelope.ErrorCode), result.code)
		result.message = first(envelope.ErrorMessage, result.message)
	case len(envelope.Detail) > 0 && (httpFailed || id == Tavily):
		var detail errorObject
		var validation []errorObject
		var message string
		switch {
		case json.Unmarshal(envelope.Detail, &detail) == nil:
			nested := detail.diagnostic()
			result = upstreamError{first(nested.code, result.code), first(nested.message, result.message)}
		case json.Unmarshal(envelope.Detail, &validation) == nil && len(validation) != 0:
			result = validation[0].diagnostic()
		case json.Unmarshal(envelope.Detail, &message) == nil:
			result.message = first(message, result.message)
		}
	}
	if httpFailed || envelope.Success != nil && !*envelope.Success || len(envelope.Errors) != 0 {
		return result, true
	}
	return result, result.code != "" && result.code != "0" && result.code != "200" && !strings.EqualFold(result.code, "success") &&
		(id == Baidu || id == Aliyun || len(envelope.ErrorCode) > 0)
}

func codeText(raw json.RawMessage) string {
	var code string
	if json.Unmarshal(raw, &code) == nil {
		return code
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var validErrorCode = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,128}$`)

func responseFailure(id ID, status int, upstream upstreamError, credential string) *Failure {
	code := upstream.code
	if !validErrorCode.MatchString(code) || code == credential {
		code = ""
	}
	failure := httpFailure(status)
	reasons := providerReasons[id]
	reason, ok := reasons[code]
	if !ok {
		reason, ok = reasons[strconv.Itoa(status)]
	}
	if ok {
		failure.Kind, failure.Message, failure.Retryable = reason.Kind, reason.Message, reason.Retryable
	}
	if message := providerdiagnostic.Text(upstream.message, credential); message != "" && !strings.EqualFold(message, code) {
		if !ok {
			failure.Message = message
		} else if !strings.EqualFold(message, failure.Message) {
			failure.Detail = message
		}
	}
	failure.Code = code
	failure.HTTPStatus = status
	return &failure
}

func httpFailure(status int) Failure {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return Failure{Kind: "invalid_request", Message: "The search service rejected the request parameters."}
	case http.StatusUnauthorized:
		return Failure{Kind: "authentication", Message: "The search service rejected the credentials."}
	case http.StatusPaymentRequired:
		return Failure{Kind: "billing", Message: "The search service requires payment or more credits."}
	case http.StatusForbidden:
		return Failure{Kind: "access_denied", Message: "The search service denied access to this request."}
	case http.StatusNotFound:
		return Failure{Kind: "not_found", Message: "The search endpoint or requested resource was not found."}
	case http.StatusMethodNotAllowed:
		return Failure{Kind: "invalid_request", Message: "The search endpoint does not support this request method."}
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return Failure{Kind: "timeout", Message: "The search service timed out while handling the request.", Retryable: true}
	case http.StatusRequestEntityTooLarge:
		return Failure{Kind: "invalid_request", Message: "The search request was too large."}
	case http.StatusUnsupportedMediaType:
		return Failure{Kind: "invalid_request", Message: "The search service does not accept the request content type."}
	case http.StatusTooManyRequests:
		return Failure{Kind: "rate_limited", Message: "The search service applied a request rate or usage limit.", Retryable: true}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return Failure{Kind: "service_unavailable", Message: "The search service reported a server or upstream error.", Retryable: true}
	}
	if status >= 500 {
		return Failure{Kind: "service_unavailable", Message: "The search service returned a server error.", Retryable: true}
	}
	if status >= 300 && status < 400 {
		return Failure{Kind: "unexpected_redirect", Message: "The search endpoint redirected the request."}
	}
	if reason := http.StatusText(status); status >= 400 && reason != "" {
		return Failure{Kind: "request_rejected", Message: reason + "."}
	}
	return Failure{Kind: "request_rejected", Message: "The search service rejected the request without providing a reason."}
}
