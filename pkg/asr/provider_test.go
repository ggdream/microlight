package asr

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testWAV(seconds int) Audio {
	data := make([]byte, 44+32000*seconds)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 16000)
	binary.LittleEndian.PutUint32(data[28:], 32000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	return Audio{Name: "sample.wav", Data: data}
}
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestProviderWireFormats(t *testing.T) {
	for _, id := range IDs() {
		t.Run(string(id), func(t *testing.T) {
			o := Options{APIKey: "test-key", SecretKey: "test-secret", AppID: "test-app", ProjectID: "project"}
			if id == Azure {
				o.BaseURL = "https://azure.example"
			}
			calls := 0
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				r.Body.Close()
				if !strings.HasPrefix(r.URL.String(), "https://") {
					t.Fatal("insecure destination")
				}
				switch id {
				case DeepSeek, OpenAI, Zhipu, Azure:
					r.Body = io.NopCloser(bytes.NewReader(raw))
					if err := r.ParseMultipartForm(2 << 20); err != nil {
						t.Fatal(err)
					}
					defer r.MultipartForm.RemoveAll()
					field := "file"
					if id == Azure {
						field = "audio"
					}
					file, _, err := r.FormFile(field)
					if err != nil {
						t.Fatal(err)
					}
					data, _ := io.ReadAll(file)
					file.Close()
					if !bytes.Equal(data, testWAV(1).Data) {
						t.Fatal("audio was changed")
					}
					if id == DeepSeek && r.FormValue("model") != "" {
						t.Fatal("DeepSeek has no model parameter")
					}
					if id != DeepSeek && id != Azure && r.FormValue("model") == "" {
						t.Fatal("missing transcription model")
					}
					if id == Azure {
						if r.Header.Get("Ocp-Apim-Subscription-Key") != "test-key" || r.FormValue("definition") != "{}" {
							t.Fatal("missing Azure definition/key")
						}
						return response(`{"combinedPhrases":[{"text":"hello"}]}`), nil
					}
					if r.Header.Get("Authorization") != "Bearer test-key" {
						t.Fatal("missing bearer credential")
					}
					return response(`{"text":"hello"}`), nil
				case Aliyun, MiMo:
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						t.Fatal("invalid JSON")
					}
					if m["model"] == nil || !bytes.Contains(raw, []byte("data:audio/wav;base64,")) {
						t.Fatal("missing audio data URI")
					}
					return response(`{"choices":[{"message":{"content":"hello"}}]}`), nil
				case Volcengine:
					if r.Header.Get("X-Api-Key") != "test-key" || r.Header.Get("X-Api-Resource-Id") != "volc.bigasr.auc_turbo" {
						t.Fatal("missing Volcengine auth/resource")
					}
					if requestID := r.Header.Get("X-Api-Request-Id"); len(requestID) != 36 || requestID[14] != '7' {
						t.Fatal("request ID must be UUIDv7")
					}
					res := response(`{"result":{"text":"hello"}}`)
					res.Header.Set("X-Api-Status-Code", "20000000")
					return res, nil
				case Tencent:
					if !strings.HasPrefix(r.Header.Get("Authorization"), "TC3-HMAC-SHA256 Credential=test-key/") || r.Header.Get("X-TC-Action") != "SentenceRecognition" {
						t.Fatal("missing Tencent signing/action")
					}
					if !bytes.Contains(raw, []byte(`"SourceType":1`)) {
						t.Fatal("not local audio upload")
					}
					return response(`{"Response":{"Result":"hello","AudioDuration":1000}}`), nil
				case Baidu:
					if calls == 1 {
						if !bytes.Contains(raw, []byte("client_secret=test-secret")) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
							t.Fatal("invalid OAuth request")
						}
						return response(`{"access_token":"temporary-secret-token"}`), nil
					}
					return response(`{"err_no":0,"result":["hello"]}`), nil
				case Xfyun:
					if r.Header.Get("signature") == "" {
						t.Fatal("missing signature")
					}
					if calls == 1 {
						if r.URL.Query().Get("durationCheckDisable") != "true" || r.URL.Query().Get("language") == "" || !bytes.Equal(raw, testWAV(1).Data) {
							t.Fatal("invalid iFLYTEK upload")
						}
						return response(`{"code":"000000","content":{"orderId":"order-1"}}`), nil
					}
					if r.URL.Query().Get("orderId") != "order-1" || string(raw) != "{}" {
						t.Fatal("invalid iFLYTEK polling")
					}
					lattice := map[string]any{"lattice": []any{map[string]any{"json_1best": `{"st":{"rt":[{"ws":[{"cw":[{"w":"hello"}]}]}]}}`}}}
					return response(string(mustJSON(map[string]any{"code": "000000", "content": map[string]any{"orderInfo": map[string]any{"status": 4}, "orderResult": string(mustJSON(lattice))}}))), nil
				case Huawei:
					if r.Header.Get("X-Auth-Token") != "test-key" || !strings.Contains(r.URL.Path, "/project/asr/short-audio") {
						t.Fatal("missing IAM/project")
					}
					return response(`{"result":{"text":"hello"}}`), nil
				case Google:
					if r.Header.Get("Authorization") != "Bearer test-key" || !bytes.Contains(raw, []byte(`"languageCode":"zh-CN"`)) {
						t.Fatal("missing OAuth/language")
					}
					return response(`{"results":[{"alternatives":[{"transcript":"hello"}]}]}`), nil
				}
				t.Fatal("unhandled provider")
				return nil, nil
			})
			p, _, err := NewProvider(id, o, &http.Client{Transport: transport})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.Transcribe(t.Context(), testWAV(1))
			if err != nil || result.Text != "hello" {
				t.Fatalf("bad transcript: %+v %v", result, err)
			}
		})
	}
}
func TestProviderFailuresArePreservedAndRedacted(t *testing.T) {
	for _, tc := range []struct {
		id     ID
		status int
		body   string
		header http.Header
	}{
		{DeepSeek, 503, `{"error":{"code":"busy","message":"queue full test-key"}}`, nil},
		{MiMo, 200, `{"error":{"code":"unknown-future-code","message":"provider explanation test-key"}}`, nil},
		{Tencent, 200, `{"Response":{"Error":{"Code":"AuthFailure","Message":"bad test-secret"}}}`, nil},
		{Volcengine, 200, `{"result":{}}`, http.Header{"X-Api-Status-Code": {"45000000"}, "X-Api-Message": {"quota exceeded test-key"}}},
		{Huawei, 200, `{"error_code":"SIS.9999","error_msg":"future diagnostic test-key"}`, nil},
	} {
		t.Run(string(tc.id), func(t *testing.T) {
			p, _, err := NewProvider(tc.id, Options{APIKey: "test-key", SecretKey: "test-secret", ProjectID: "project"}, &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				r.Body.Close()
				res := response(tc.body)
				res.StatusCode = tc.status
				if tc.header != nil {
					res.Header = tc.header
				}
				return res, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Transcribe(t.Context(), testWAV(1))
			if err == nil {
				t.Fatal("provider error was swallowed")
			}
			failure := DescribeFailure(err)
			raw := string(mustJSON(failure))
			if strings.Contains(raw, "test-key") || strings.Contains(raw, "test-secret") || failure.Detail == "" || failure.Code == "" {
				t.Fatalf("incorrect redacted error: %s", raw)
			}
		})
	}
}
func TestLimitsCancellationAndRedirects(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("unexpected network") })}
	p, _, _ := NewProvider(DeepSeek, Options{APIKey: "key"}, client)
	if _, err := p.Transcribe(t.Context(), Audio{Name: "a.mp3", Data: []byte("x")}); err == nil {
		t.Fatal("unsupported format accepted")
	}
	if _, err := p.Transcribe(t.Context(), testWAV(121)); err == nil {
		t.Fatal("duration limit ignored")
	}
	if calls != 0 {
		t.Fatal("invalid file was uploaded")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := wait(canceled, time.Hour); err != context.Canceled {
		t.Fatal(err)
	}
	s := p.(*service)
	if err := s.client.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatal("redirect may leak credentials")
	}
	if _, _, err := NewProvider(DeepSeek, Options{APIKey: "key", BaseURL: "https://user:pass@example.org"}, nil); err == nil {
		t.Fatal("credential URL accepted")
	}
}
