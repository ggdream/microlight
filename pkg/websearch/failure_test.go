package websearch

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProvidersPreserveSearchErrors(t *testing.T) {
	const privateKey = "sample-private-key"
	for _, tc := range []struct {
		id                       ID
		status                   int
		body, code, kind, reason string
		retryable                bool
	}{
		{DuckDuckGo, 429, "<html>blocked</html>", "", "rate_limited", "limit", true},
		{SearXNG, 403, "<html>Forbidden</html>", "", "access_denied", "JSON output", false},
		{Google, 403, `{"error":{"code":403,"message":"Daily Limit Exceeded","errors":[{"reason":"dailyLimitExceeded"}]}}`, "dailyLimitExceeded", "daily_quota", "Daily Limit Exceeded", false},
		{Wikipedia, 200, `{"error":{"code":"future-wiki-code","info":"Search index unavailable"}}`, "future-wiki-code", "request_rejected", "Search index unavailable", false},
		{Tavily, 422, `{"detail":[{"type":"string_type","msg":"Query must be a string","input":"sample-private-key"}]}`, "string_type", "invalid_request", "Query must be a string", false},
		{Tavily, 433, `{}`, "", "billing_limit", "pay-as-you-go spending limit", false},
		{Firecrawl, 402, `{"success":false,"code":"NEW_BILLING_CODE","error":"Out of credits","details":[{"message":"unrelated"}]}`, "NEW_BILLING_CODE", "billing", "Out of credits", false},
		{DeepSeek, 400, `{"error":{"code":"invalid_request_error","message":"Endpoint https://example.com/?key=signed-value rejected sample-private-key"}}`, "invalid_request_error", "invalid_request", "Endpoint [redacted] rejected [redacted]", false},
		{Baidu, 429, `{"code":"QUOTA_USER_DAILY_FREE"}`, "QUOTA_USER_DAILY_FREE", "daily_quota", "daily free quota", false},
		{Baidu, 200, `{"code":"InvalidArgument","message":"Query cannot be empty"}`, "InvalidArgument", "invalid_request", "Query cannot be empty", false},
		{Baidu, 401, `{"code":216003,"message":"Authentication error"}`, "216003", "authentication", "Authentication error", false},
		{Aliyun, 403, `{"Code":"Retrieval.NotActivate"}`, "Retrieval.NotActivate", "service_not_enabled", "has not been activated", false},
		{Tencent, 200, `{"Response":{"Error":{"Code":"RequestLimitExceeded","Message":"Concurrent search limit exceeded"}}}`, "RequestLimitExceeded", "rate_limited", "Concurrent search limit exceeded", true},
		{Volcengine, 200, `{"ResponseMetadata":{"Error":{"CodeN":10412}},"Result":null}`, "10412", "quota", "subscription quota", false},
	} {
		t.Run(string(tc.id)+"/"+tc.code, func(t *testing.T) {
			client := &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
				if req.Body != nil {
					req.Body.Close()
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			provider, _, err := NewProvider(t.Context(), tc.id, Options{
				APIKey: privateKey, SearchEngineID: "test-cx", BaseURL: "https://searx.example.test",
			}, client)
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Search(t.Context(), "test search")
			if err == nil {
				t.Fatal("expected a search error")
			}
			failure := DescribeFailure(err)
			description := failure.Message + " " + failure.Detail
			if failure.Code != tc.code || failure.Kind != tc.kind || failure.HTTPStatus != tc.status ||
				failure.Retryable != tc.retryable || !strings.Contains(description, tc.reason) ||
				strings.Contains(description, privateKey) || strings.Contains(description, "signed-value") {
				t.Errorf("unexpected search failure: %+v", failure)
			}
		})
	}
}
