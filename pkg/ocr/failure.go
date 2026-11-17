package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	paddle "github.com/PaddlePaddle/PaddleOCR/api_sdk/go"
	"github.com/ggdream/microlight/internal/providerdiagnostic"
)

// Failure describes an OCR error without exposing raw responses, credentials, or signed URLs.
type Failure struct {
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
	Code       int    `json:"code,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Retryable  bool   `json:"retryable"`
}

func (f *Failure) Error() string { return f.Message }

func DescribeFailure(err error) Failure {
	if errors.Is(err, context.Canceled) {
		return Failure{Kind: "canceled", Message: "The OCR request was canceled."}
	}
	if failure, ok := errors.AsType[*Failure](err); ok {
		return *failure
	}
	switch {
	case hasError[*paddle.PollTimeoutError](err):
		return Failure{Kind: "timeout", Message: "AI Studio accepted the job, but it did not finish before the polling deadline."}
	case hasError[*paddle.RequestTimeoutError](err):
		return Failure{Kind: "timeout", Message: "A request to PaddleOCR timed out. The job status may be unknown."}
	case errors.Is(err, context.DeadlineExceeded):
		return Failure{Kind: "timeout", Message: "Timed out before receiving the OCR result."}
	case hasError[*paddle.AuthError](err):
		return Failure{Kind: "access_denied", Message: "PaddleOCR denied access to the request or result."}
	case hasError[*paddle.JobFailedError](err):
		return Failure{Kind: "job_failed", Message: "AI Studio accepted the job, but could not parse the file."}
	case hasError[*paddle.NetworkError](err):
		return Failure{Kind: "network", Message: "Could not reach the PaddleOCR service or download its result.", Retryable: true}
	case hasError[*paddle.ResponseFormatError](err), hasError[*paddle.ResultParseError](err):
		return Failure{Kind: "invalid_response", Message: "PaddleOCR returned a response that could not be read."}
	}
	if rate, ok := errors.AsType[*paddle.RateLimitError](err); ok {
		return apiFailure(rate.StatusCode, 0)
	}
	if unavailable, ok := errors.AsType[*paddle.ServiceUnavailableError](err); ok {
		return apiFailure(unavailable.StatusCode, 0)
	}
	if apiErr, ok := errors.AsType[*paddle.APIError](err); ok {
		return apiFailure(apiErr.StatusCode, 0)
	}
	return Failure{Kind: "processing_failed", Message: "PaddleOCR processing failed without a usable result."}
}

func hasError[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}

func apiFailure(status, code int) Failure {
	type reason struct {
		kind, message string
		retryable     bool
	}
	var detail reason
	switch code {
	case 401:
		detail = reason{"authentication", "AI Studio rejected the access token.", false}
	case 500:
		detail = reason{"service_unavailable", "AI Studio reported an internal error.", true}
	case 10001:
		detail = reason{"empty_file", "AI Studio received an empty file.", false}
	case 10002:
		detail = reason{"invalid_source", "AI Studio could not read the specified file URL.", false}
	case 10003:
		detail = reason{"file_too_large", "The file exceeds AI Studio's size limit.", false}
	case 10004:
		detail = reason{"unsupported_format", "AI Studio does not support this file format.", false}
	case 10005:
		detail = reason{"invalid_file", "AI Studio could not parse the file contents.", false}
	case 10006:
		detail = reason{"page_limit", "The file exceeds AI Studio's page limit.", false}
	case 10007:
		detail = reason{"invalid_model", "AI Studio did not accept the selected model.", false}
	case 10008:
		detail = reason{"invalid_request", "AI Studio rejected the request parameters.", false}
	case 10009:
		detail = reason{"batch_limit", "The AI Studio batch has reached its limit of 100 jobs.", false}
	case 10010:
		detail = reason{"queue_full", "AI Studio's submission queue is full. Try again later.", true}
	case 11001:
		detail = reason{"job_missing", "AI Studio could not find the submitted job.", false}
	case 11002:
		detail = reason{"job_expired", "The AI Studio job expired before its result could be read.", false}
	case 11003:
		detail = reason{"job_failed", "AI Studio accepted the job, but could not parse the file.", false}
	case 12001:
		detail = reason{"daily_quota", "AI Studio's daily page quota has been reached.", false}
	case 12002:
		detail = reason{"rate_limited", "AI Studio received too many requests in a short time. Try again later.", true}
	default:
		switch {
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			detail = reason{"access_denied", "AI Studio denied access. The token may be invalid or lack permission.", false}
		case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
			detail = reason{"invalid_request", "AI Studio rejected the request parameters.", false}
		case status == http.StatusNotFound:
			detail = reason{"not_found", "AI Studio could not find the requested endpoint or resource.", false}
		case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
			detail = reason{"timeout", "AI Studio timed out while handling the request.", true}
		case status == http.StatusServiceUnavailable:
			detail = reason{"service_unavailable", "AI Studio is temporarily unavailable or handling too many requests.", true}
		case status == http.StatusBadGateway:
			detail = reason{"service_unavailable", "The AI Studio gateway received an invalid upstream response.", true}
		case status == http.StatusTooManyRequests:
			detail = reason{"rate_limited", "AI Studio applied a request or usage limit.", true}
		case status == http.StatusRequestEntityTooLarge:
			detail = reason{"file_too_large", "The request exceeds AI Studio's size limit.", false}
		case status == http.StatusUnsupportedMediaType:
			detail = reason{"unsupported_format", "AI Studio does not support this file format.", false}
		case status >= 500:
			detail = reason{"service_unavailable", "AI Studio returned a server error.", true}
		default:
			detail = reason{"request_rejected", "AI Studio rejected the request without a documented reason.", false}
			if message := http.StatusText(status); status >= 400 && message != "" {
				detail.message = message + "."
			}
		}
	}
	return Failure{Kind: detail.kind, Message: detail.message, Code: code, HTTPStatus: status, Retryable: detail.retryable}
}

type paddleTransport struct{ http.RoundTripper }

// The SDK drops business error codes when decoding its errors. Capture the code
// in the AI Studio job response before that happens. Leave successful bodies
// and external result downloads to the SDK.
func (t paddleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	const jobs = "/api/v2/ocr/jobs"
	if err != nil || req.URL.Scheme+"://"+req.URL.Host != paddle.DefaultBaseURL ||
		(req.URL.Path != jobs && !strings.HasPrefix(req.URL.Path, jobs+"/")) {
		return resp, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return resp, nil
	}
	const maxEnvelope = 32 << 10
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelope+1))
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("read AI Studio response: %w", err)
	}
	var envelope struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
		Data    struct {
			State    string `json:"state"`
			ErrorMsg string `json:"errorMsg"`
		} `json:"data"`
	}
	// Batch responses may use a different data shape. The code remains on the envelope.
	_ = json.Unmarshal(body, &envelope)
	if envelope.Code != 0 || resp.StatusCode >= 400 || envelope.Data.State == "failed" {
		resp.Body.Close()
		failure := apiFailure(resp.StatusCode, envelope.Code)
		if envelope.Code == 0 && envelope.Data.State == "failed" {
			failure.Kind = "job_failed"
			failure.Message = "AI Studio accepted the job, but processing failed."
			failure.Retryable = false
		}
		message := envelope.Data.ErrorMsg
		if message == "" {
			message = envelope.Msg
		}
		if message == "" {
			message = envelope.Message
		}
		credential := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		failure.Detail = providerdiagnostic.Text(message, credential)
		return nil, &failure
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(body), resp.Body), resp.Body}
	return resp, nil
}
