package localtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/ggdream/microlight/pkg/ocr"
)

type ocrTransportFunc func(*http.Request) (*http.Response, error)

func (send ocrTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return send(r) }

func TestOCRToolsReturnAIStudioFailureToModel(t *testing.T) {
	const token = "fixture-private-token"
	for _, tc := range []struct {
		name, kind   string
		status, code int
		retryable    bool
	}{
		{OCR, "queue_full", http.StatusBadRequest, 10010, true},
		{ParseDocument, "daily_quota", http.StatusOK, 12001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := ocrTransportFunc(func(req *http.Request) (*http.Response, error) {
				req.Body.Close()
				body := fmt.Sprintf(`{"code":%d,"msg":"remote diagnostic %s"}`, tc.code, token)
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			tools, err := NewOCRTools(OCRConfig{Enabled: true, Token: token}, transport)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(t.Context(), approvedOperationKey{}, localOperation{
				name: tc.name, callID: "call-1", ocrData: []byte("test input"), ocrExtension: ".png",
			})
			var reported Result
			ctx = WithResultReporter(ctx, func(result Result) { reported = result })
			for _, base := range tools {
				info, err := base.Info(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if info.Name != tc.name {
					continue
				}
				result, err := base.(tool.InvokableTool).InvokableRun(ctx, "")
				var decoded struct {
					Status string
					Model  string
					Error  ocr.Failure
				}
				if err != nil || json.Unmarshal([]byte(result), &decoded) != nil || decoded.Status != "error" ||
					decoded.Error.Kind != tc.kind || decoded.Error.Code != tc.code || decoded.Error.HTTPStatus != tc.status ||
					decoded.Error.Retryable != tc.retryable || decoded.Error.Message == "" ||
					decoded.Error.Detail != "remote diagnostic [redacted]" ||
					strings.Contains(result, token) || reported.Text != result || reported.Model != decoded.Model {
					t.Fatalf("incorrect model error or preview: %s, %v", strings.ReplaceAll(result, token, "[redacted]"), err)
				}
				return
			}
			t.Fatalf("tool %s not registered", tc.name)
		})
	}
}
