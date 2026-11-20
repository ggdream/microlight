// Package asr provides file-based speech recognition independently of agent/UI code.
package asr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type ID string

const (
	DeepSeek   ID = "deepseek"
	Aliyun     ID = "aliyun"
	Volcengine ID = "volcengine"
	Tencent    ID = "tencent"
	Baidu      ID = "baidu"
	Xfyun      ID = "xfyun"
	Huawei     ID = "huawei"
	MiMo       ID = "mimo"
	Zhipu      ID = "zhipu"
	OpenAI     ID = "openai"
	Google     ID = "google"
	Azure      ID = "azure"
)

// Credentials belong to a service, rather than to an individual tool invocation.
type Options struct {
	APIKey     string `json:"api_key,omitempty"`
	SecretKey  string `json:"secret_key,omitempty"`
	AppID      string `json:"app_id,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Language   string `json:"language,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
}
type Audio struct {
	Name string
	Data []byte
}
type Segment struct {
	Text    string `json:"text"`
	StartMS int64  `json:"start_ms,omitempty"`
	EndMS   int64  `json:"end_ms,omitempty"`
	Speaker string `json:"speaker,omitempty"`
}
type Result struct {
	Text       string    `json:"text"`
	DurationMS int64     `json:"duration_ms,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	Segments   []Segment `json:"segments,omitempty"`
	Truncated  bool      `json:"truncated,omitempty"`
}
type Provider interface {
	Transcribe(context.Context, Audio) (Result, error)
}
type Definition struct {
	ID                                                     ID
	Name, Endpoint, Model                                  string
	Formats                                                []string
	MaxBytes                                               int
	MaxSeconds                                             int
	SecretKey, AppID, ProjectID, BaseURL, EndpointRequired bool
}

const MaxBytes = 100 << 20

var definitions = []Definition{
	{ID: DeepSeek, Name: "DeepSeek", Endpoint: "https://api.deepseek.com/v1/tools/asr", Formats: []string{"wav", "ogg"}, MaxBytes: 8 << 20, MaxSeconds: 120},
	{ID: Aliyun, BaseURL: true, Name: "Alibaba Cloud", Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", Model: "qwen3-asr-flash", Formats: []string{"wav", "mp3", "m4a", "aac", "ogg", "flac", "opus"}, MaxBytes: 7 << 20, MaxSeconds: 300},
	{ID: Volcengine, Name: "Volcengine", Endpoint: "https://openspeech.bytedance.com/api/v3/auc/bigmodel/recognize/flash", Model: "volc.bigasr.auc_turbo", Formats: []string{"wav", "mp3", "ogg"}, MaxBytes: MaxBytes, MaxSeconds: 7200},
	{ID: Tencent, Name: "Tencent Cloud", Endpoint: "https://asr.tencentcloudapi.com/", Model: "16k_zh", SecretKey: true, Formats: []string{"wav", "pcm", "ogg", "opus", "speex", "silk", "mp3", "m4a", "aac", "amr"}, MaxBytes: 3_000_000, MaxSeconds: 60},
	{ID: Baidu, Name: "Baidu AI Cloud", Endpoint: "https://vop.baidu.com/server_api", Model: "1537", SecretKey: true, Formats: []string{"wav", "pcm", "amr", "m4a"}, MaxBytes: 8 << 20, MaxSeconds: 60},
	{ID: Xfyun, Name: "iFLYTEK", Endpoint: "https://office-api-ist-dx.iflyaisol.com", SecretKey: true, AppID: true, Formats: []string{"mp3", "wav", "pcm", "opus", "flac", "ogg", "speex"}, MaxBytes: MaxBytes, MaxSeconds: 18000},
	{ID: Huawei, BaseURL: true, Name: "Huawei Cloud SIS", Endpoint: "https://sis-ext.cn-north-4.myhuaweicloud.com", Model: "chinese_16k_common", ProjectID: true, Formats: []string{"wav", "mp3", "pcm", "ogg"}, MaxBytes: 3_000_000, MaxSeconds: 60},
	{ID: MiMo, Name: "Xiaomi MiMo", Endpoint: "https://api.xiaomimimo.com/v1/chat/completions", Model: "mimo-v2.5-asr", Formats: []string{"wav", "mp3"}, MaxBytes: 7_500_000},
	{ID: Zhipu, Name: "Zhipu AI", Endpoint: "https://open.bigmodel.cn/api/paas/v4/audio/transcriptions", Model: "glm-asr-2512", Formats: []string{"wav", "mp3"}, MaxBytes: 25 << 20, MaxSeconds: 30},
	{ID: OpenAI, Name: "OpenAI", Endpoint: "https://api.openai.com/v1/audio/transcriptions", Model: "gpt-transcribe", Formats: []string{"wav", "mp3", "mp4", "m4a", "mpeg", "mpga", "webm", "ogg", "flac"}, MaxBytes: 25 << 20},
	{ID: Google, Name: "Google Cloud Speech-to-Text", Endpoint: "https://speech.googleapis.com/v1/speech:recognize", Model: "latest_long", Formats: []string{"wav", "flac", "mp3", "ogg", "webm"}, MaxBytes: 10_000_000, MaxSeconds: 60},
	{ID: Azure, BaseURL: true, Name: "Azure Speech", EndpointRequired: true, Formats: []string{"wav", "mp3", "mp4", "m4a", "aac", "ogg", "flac", "wma", "webm"}, MaxBytes: MaxBytes, MaxSeconds: 18000},
}

func IDs() []ID {
	ids := make([]ID, 0, len(definitions))
	for _, d := range definitions {
		ids = append(ids, d.ID)
	}
	return ids
}
func Lookup(id ID) (Definition, bool) {
	for _, d := range definitions {
		if d.ID == id {
			d.Formats = slices.Clone(d.Formats)
			return d, true
		}
	}
	return Definition{}, false
}
func (d Definition) Validate(o Options, required bool) error {
	for _, v := range []string{o.APIKey, o.SecretKey, o.AppID, o.BaseURL, o.Model, o.ProjectID, o.Language} {
		if strings.ContainsAny(v, "\r\n\x00") {
			return errors.New("ASR configuration contains an invalid control character")
		}
	}
	if required {
		for name, v := range map[string]string{"API Key / Token": o.APIKey} {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%s: %s is required", d.Name, name)
			}
		}
		if d.SecretKey && strings.TrimSpace(o.SecretKey) == "" {
			return fmt.Errorf("%s: Secret Key is required", d.Name)
		}
		if d.AppID && strings.TrimSpace(o.AppID) == "" {
			return fmt.Errorf("%s: App ID is required", d.Name)
		}
		if d.ProjectID && strings.TrimSpace(o.ProjectID) == "" {
			return fmt.Errorf("%s: Project ID is required", d.Name)
		}
	}
	if o.BaseURL != "" || d.EndpointRequired && required {
		u, e := url.Parse(o.BaseURL)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("ASR endpoint must be an HTTPS URL without credentials, query or fragment")
		}
	}
	if o.SampleRate < 0 || o.SampleRate > 192000 {
		return errors.New("invalid ASR sample rate")
	}
	if d.ProjectID && strings.ContainsAny(o.ProjectID, "/\\?#") {
		return errors.New("invalid ASR Project ID")
	}
	return nil
}
func NewProvider(id ID, options Options, client *http.Client) (Provider, string, error) {
	d, ok := Lookup(id)
	if !ok {
		return nil, "", errors.New("unknown ASR provider")
	}
	if e := d.Validate(options, true); e != nil {
		return nil, "", e
	}
	endpoint := d.Endpoint
	if options.BaseURL != "" {
		endpoint = strings.TrimRight(options.BaseURL, "/")
	}
	if options.Model == "" {
		options.Model = d.Model
	}
	if options.SampleRate == 0 {
		options.SampleRate = 16000
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &service{definition: d, options: options, endpoint: endpoint, client: &bounded}, endpoint, nil
}
