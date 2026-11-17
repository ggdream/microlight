package ocr

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	paddle "github.com/PaddlePaddle/PaddleOCR/api_sdk/go"
)

func TestDocumentedAIStudioErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		code, status int
		kind         string
		retryable    bool
	}{
		{401, 401, "authentication", false},
		{500, 500, "service_unavailable", true},
		{10001, 400, "empty_file", false},
		{10002, 400, "invalid_source", false},
		{10003, 400, "file_too_large", false},
		{10004, 400, "unsupported_format", false},
		{10005, 400, "invalid_file", false},
		{10006, 400, "page_limit", false},
		{10007, 400, "invalid_model", false},
		{10008, 400, "invalid_request", false},
		{10009, 400, "batch_limit", false},
		{10010, 400, "queue_full", true},
		{11001, 404, "job_missing", false},
		{11002, 400, "job_expired", false},
		{11003, 200, "job_failed", false},
		{12001, 403, "daily_quota", false},
		{12002, 429, "rate_limited", true},
		{19999, 200, "request_rejected", false},
		{0, 200, "job_failed", false},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, paddle.DefaultBaseURL+"/api/v2/ocr/jobs/test", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer test-private-token")
			transport := paddleTransport{failureTransport(func(*http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"code":%d,"msg":"generic","data":{"state":"failed","errorMsg":"PDF is encrypted; test-private-token https://example.test/signed?key=secret"}}`, tc.code)
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			_, err = transport.RoundTrip(req)
			got := DescribeFailure(err)
			if got.Code != tc.code || got.HTTPStatus != tc.status || got.Kind != tc.kind ||
				got.Retryable != tc.retryable || got.Message == "" || got.Detail != "PDF is encrypted; [redacted] [redacted]" {
				t.Errorf("unexpected OCR failure: %+v", got)
			}
		})
	}
}

type failureTransport func(*http.Request) (*http.Response, error)

func (transport failureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}
