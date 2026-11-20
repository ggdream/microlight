package asr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"uuid"
)

func dataURI(a Audio) string {
	return "data:audio/" + Format(a) + ";base64," + base64.StdEncoding.EncodeToString(a.Data)
}
func (s *service) transcriptions(ctx context.Context, a Audio) (Result, error) {
	fields := map[string]string{}
	if s.definition.ID != DeepSeek {
		fields["model"] = s.options.Model
	}
	if s.definition.ID == Zhipu {
		fields["stream"] = "false"
	}
	if s.definition.ID == OpenAI {
		fields["response_format"] = "json"
		if s.options.Language != "" {
			fields["language"] = s.options.Language
		}
	}
	body, err := s.multipart(ctx, "file", a, fields, bearer(s.options.APIKey))
	if err != nil {
		return Result{}, err
	}
	return Result{Text: scalar(body["text"]), DurationMS: number(body["duration_ms"]), RequestID: scalar(body["request_id"])}, nil
}
func (s *service) chat(ctx context.Context, a Audio) (Result, error) {
	audio := map[string]any{"data": dataURI(a)}
	if s.definition.ID == MiMo {
		audio["format"] = Format(a)
	}
	payload := map[string]any{"model": s.options.Model, "stream": false, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_audio", "input_audio": audio}}}}}
	if s.options.Language != "" {
		payload["asr_options"] = map[string]any{"language": s.options.Language}
	}
	body, _, err := s.post(ctx, s.endpoint, payload, bearer(s.options.APIKey))
	if err != nil {
		return Result{}, err
	}
	choices := array(body["choices"])
	if len(choices) == 0 {
		return Result{}, s.failure(200, "missing_choices", "ASR response has no choices", "", mustJSON(body))
	}
	content := at(object(choices[0]), "message", "content")
	text, _ := content.(string)
	if text == "" {
		for _, part := range array(content) {
			text += scalar(object(part)["text"])
		}
	}
	return Result{Text: text, RequestID: scalar(body["id"])}, nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *service) volcengine(ctx context.Context, a Audio) (Result, error) {
	headers := map[string]string{"X-Api-Key": s.options.APIKey, "X-Api-Resource-Id": s.options.Model, "X-Api-Request-Id": uuid.NewV7().String(), "X-Api-Sequence": "-1"}
	body, h, err := s.post(ctx, s.endpoint, map[string]any{"user": map[string]any{"uid": "microlight"}, "audio": map[string]any{"data": base64.StdEncoding.EncodeToString(a.Data)}, "request": map[string]any{"model_name": "bigmodel"}}, headers)
	if err != nil {
		return Result{}, err
	}
	code := h.Get("X-Api-Status-Code")
	if code != "20000000" {
		return Result{}, s.failure(200, code, h.Get("X-Api-Message"), h.Get("X-Tt-Logid"), mustJSON(body))
	}
	result := object(body["result"])
	r := Result{Text: scalar(result["text"]), DurationMS: number(at(body, "audio_info", "duration")), RequestID: h.Get("X-Tt-Logid")}
	for _, v := range array(result["utterances"]) {
		u := object(v)
		r.Segments = append(r.Segments, Segment{Text: scalar(u["text"]), StartMS: number(u["start_time"]), EndMS: number(u["end_time"]), Speaker: scalar(at(u, "additions", "speaker"))})
	}
	return r, nil
}
func (s *service) baidu(ctx context.Context, a Audio) (Result, error) {
	// OAuth credentials never enter query strings, application logs or tool arguments.
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {s.options.APIKey}, "client_secret": {s.options.SecretKey}}
	token, _, err := s.do(ctx, "POST", "https://aip.baidubce.com/oauth/2.0/token", "application/x-www-form-urlencoded", []byte(form.Encode()), nil)
	if err != nil {
		return Result{}, err
	}
	access := scalar(token["access_token"])
	if access == "" {
		return Result{}, s.failure(200, "missing_token", "Baidu did not return an access token", "", mustJSON(token))
	}
	authenticated := *s
	authenticated.redactions = append([]string{access}, s.redactions...)
	s = &authenticated
	model, err := strconv.Atoi(s.options.Model)
	if err != nil {
		return Result{}, fmt.Errorf("Baidu dev_pid must be an integer")
	}
	body, _, err := s.post(ctx, s.endpoint, map[string]any{"format": Format(a), "rate": s.audioRate(a), "channel": 1, "cuid": "microlight", "token": access, "dev_pid": model, "speech": base64.StdEncoding.EncodeToString(a.Data), "len": len(a.Data)}, nil)
	if err != nil {
		return Result{}, err
	}
	if code := scalar(body["err_no"]); code != "0" {
		return Result{}, s.failure(200, code, scalar(body["err_msg"]), scalar(body["sn"]), mustJSON(body))
	}
	var texts []string
	for _, v := range array(body["result"]) {
		texts = append(texts, scalar(v))
	}
	return Result{Text: strings.Join(texts, "\n"), RequestID: scalar(body["sn"])}, nil
}
func (s *service) audioRate(a Audio) int {
	if Format(a) == "wav" {
		rate, _, _, _, err := wavInfo(a.Data)
		if err == nil {
			return rate
		}
	}
	return s.options.SampleRate
}
func (s *service) huawei(ctx context.Context, a Audio) (Result, error) {
	body, _, err := s.post(ctx, s.endpoint+"/v1/"+s.options.ProjectID+"/asr/short-audio", map[string]any{"data": base64.StdEncoding.EncodeToString(a.Data), "config": map[string]any{"audio_format": Format(a), "property": s.options.Model}}, map[string]string{"X-Auth-Token": s.options.APIKey})
	if err != nil {
		return Result{}, err
	}
	return Result{Text: scalar(at(body, "result", "text"))}, nil
}
func (s *service) google(ctx context.Context, a Audio) (Result, error) {
	language := s.options.Language
	if language == "" {
		language = "zh-CN"
	}
	config := map[string]any{"languageCode": language, "model": s.options.Model, "enableAutomaticPunctuation": true}
	switch Format(a) {
	case "wav", "flac": // Self-describing formats let the service read their encoding and rate.
	case "mp3":
		config["encoding"] = "MP3"
		config["sampleRateHertz"] = s.options.SampleRate
	case "ogg":
		config["encoding"] = "OGG_OPUS"
		config["sampleRateHertz"] = s.options.SampleRate
	case "webm":
		config["encoding"] = "WEBM_OPUS"
		config["sampleRateHertz"] = s.options.SampleRate
	}
	body, _, err := s.post(ctx, s.endpoint, map[string]any{"config": config, "audio": map[string]any{"content": base64.StdEncoding.EncodeToString(a.Data)}}, bearer(s.options.APIKey))
	if err != nil {
		return Result{}, err
	}
	var texts []string
	for _, v := range array(body["results"]) {
		alternatives := array(object(v)["alternatives"])
		if len(alternatives) > 0 {
			texts = append(texts, scalar(object(alternatives[0])["transcript"]))
		}
	}
	return Result{Text: strings.Join(texts, "\n")}, nil
}
func (s *service) azure(ctx context.Context, a Audio) (Result, error) {
	copy := *s
	copy.endpoint = strings.TrimRight(s.endpoint, "/") + "/speechtotext/transcriptions:transcribe?api-version=2025-10-15"
	definition := map[string]any{}
	if s.options.Language != "" {
		definition["locales"] = []string{s.options.Language}
	}
	body, err := copy.multipart(ctx, "audio", a, map[string]string{"definition": string(mustJSON(definition))}, map[string]string{"Ocp-Apim-Subscription-Key": s.options.APIKey})
	if err != nil {
		return Result{}, err
	}
	r := Result{DurationMS: number(body["durationMilliseconds"])}
	for _, v := range array(body["combinedPhrases"]) {
		if r.Text != "" {
			r.Text += "\n"
		}
		r.Text += scalar(object(v)["text"])
	}
	return r, nil
}
