package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Failure struct {
	Provider  ID     `json:"provider"`
	Status    int    `json:"http_status,omitempty"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func (f *Failure) Error() string {
	return fmt.Sprintf("%s ASR: %s (HTTP %d, code %s)", f.Provider, f.Message, f.Status, f.Code)
}
func DescribeFailure(err error) Failure {
	var failure *Failure
	if errors.As(err, &failure) {
		return *failure
	}
	return Failure{Message: err.Error()}
}

type service struct {
	definition Definition
	options    Options
	endpoint   string
	client     *http.Client
	redactions []string
}

func (s *service) clean(text string) string {
	for _, secret := range append([]string{s.options.APIKey, s.options.SecretKey}, s.redactions...) {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
			text = strings.ReplaceAll(text, url.QueryEscape(secret), "[redacted]")
		}
	}
	if len(text) > 16384 {
		text = text[:16384] + "… [truncated]"
	}
	return text
}
func (s *service) failure(status int, code, message, requestID string, raw []byte) error {
	return &Failure{Provider: s.definition.ID, Status: status, Code: code, Message: s.clean(message), RequestID: requestID, Detail: s.clean(string(raw))}
}
func (s *service) do(ctx context.Context, method, endpoint, contentType string, data []byte, headers map[string]string) (map[string]any, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, nil, errors.New("invalid ASR request")
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, s.failure(0, "transport", s.clean(err.Error()), "", nil)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20+1))
	if err != nil {
		return nil, res.Header, err
	}
	if len(raw) > 8<<20 {
		return nil, res.Header, s.failure(res.StatusCode, "response_too_large", "ASR response exceeds 8 MiB", "", nil)
	}
	var body map[string]any
	decodeErr := json.Unmarshal(raw, &body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code, message := scalar(body["code"]), scalar(body["message"])
		if nested := object(body["error"]); len(nested) != 0 {
			code, message = scalar(nested["code"]), scalar(nested["message"])
		}
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return nil, res.Header, s.failure(res.StatusCode, code, message, res.Header.Get("X-Request-Id"), raw)
	}
	if decodeErr != nil || body == nil {
		return nil, res.Header, s.failure(res.StatusCode, "invalid_response", "Provider returned an invalid JSON object", "", raw)
	}
	// Preserve every provider-reported error, including unknown future codes.
	if e, ok := body["error"]; ok && e != nil {
		message := scalar(object(e)["message"])
		if message == "" {
			message = scalar(e)
		}
		return nil, res.Header, s.failure(res.StatusCode, scalar(object(e)["code"]), message, scalar(body["request_id"]), raw)
	}
	if code := scalar(body["code"]); code != "" && code != "0" && code != "000000" && code != "20000000" {
		message := scalar(body["message"])
		if message == "" {
			message = scalar(body["descInfo"])
		}
		return nil, res.Header, s.failure(res.StatusCode, code, message, scalar(body["request_id"]), raw)
	}
	if code := scalar(body["error_code"]); code != "" && code != "0" {
		return nil, res.Header, s.failure(res.StatusCode, code, scalar(body["error_msg"]), scalar(body["request_id"]), raw)
	}
	return body, res.Header, nil
}
func (s *service) post(ctx context.Context, endpoint string, payload any, headers map[string]string) (map[string]any, http.Header, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return s.do(ctx, "POST", endpoint, "application/json", data, headers)
}
func (s *service) multipart(ctx context.Context, field string, a Audio, fields map[string]string, headers map[string]string) (map[string]any, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	file, err := writer.CreateFormFile(field, a.Name)
	if err != nil {
		return nil, err
	}
	if _, err = file.Write(a.Data); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	result, _, err := s.do(ctx, "POST", s.endpoint, writer.FormDataContentType(), body.Bytes(), headers)
	return result, err
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func scalar(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func number(v any) int64 { n, _ := v.(float64); return int64(n) }
func at(m map[string]any, keys ...string) any {
	var v any = m
	for _, key := range keys {
		v = object(v)[key]
	}
	return v
}
func bearer(key string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + strings.TrimSpace(key)}
}
func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *service) Transcribe(ctx context.Context, a Audio) (Result, error) {
	if err := s.definition.ValidateAudio(a); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	var r Result
	var err error
	switch s.definition.ID {
	case DeepSeek, OpenAI, Zhipu:
		r, err = s.transcriptions(ctx, a)
	case Aliyun, MiMo:
		r, err = s.chat(ctx, a)
	case Volcengine:
		r, err = s.volcengine(ctx, a)
	case Tencent:
		r, err = s.tencent(ctx, a)
	case Baidu:
		r, err = s.baidu(ctx, a)
	case Xfyun:
		r, err = s.xfyun(ctx, a)
	case Huawei:
		r, err = s.huawei(ctx, a)
	case Google:
		r, err = s.google(ctx, a)
	case Azure:
		r, err = s.azure(ctx, a)
	}
	if err == nil && strings.TrimSpace(r.Text) == "" {
		return r, s.failure(200, "empty_transcript", "Provider returned no transcript", r.RequestID, nil)
	}
	return r, err
}
