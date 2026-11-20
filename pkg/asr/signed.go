package asr

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func hmac256(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}
func sha256hex(value []byte) string { h := sha256.Sum256(value); return hex.EncodeToString(h[:]) }
func tencentAuthorization(secretID, secretKey, host, body string, now time.Time) string {
	date := now.UTC().Format("2006-01-02")
	scope := date + "/asr/tc3_request"
	canonical := "POST\n/\n\ncontent-type:application/json\nhost:" + host + "\n\ncontent-type;host\n" + sha256hex([]byte(body))
	toSign := "TC3-HMAC-SHA256\n" + strconv.FormatInt(now.Unix(), 10) + "\n" + scope + "\n" + sha256hex([]byte(canonical))
	key := hmac256([]byte("TC3"+secretKey), date)
	key = hmac256(key, "asr")
	key = hmac256(key, "tc3_request")
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + scope + ", SignedHeaders=content-type;host, Signature=" + hex.EncodeToString(hmac256(key, toSign))
}
func (s *service) tencent(ctx context.Context, a Audio) (Result, error) {
	format := Format(a)
	if format == "ogg" {
		format = "ogg-opus"
	}
	payload := mustJSON(map[string]any{"SubServiceType": 2, "ProjectId": 0, "EngSerViceType": s.options.Model, "VoiceFormat": format, "SourceType": 1, "Data": base64.StdEncoding.EncodeToString(a.Data), "DataLen": len(a.Data)})
	u, _ := url.Parse(s.endpoint)
	now := time.Now()
	headers := map[string]string{"Authorization": tencentAuthorization(s.options.APIKey, s.options.SecretKey, u.Host, string(payload), now), "X-TC-Timestamp": strconv.FormatInt(now.Unix(), 10), "X-TC-Version": "2019-06-14", "X-TC-Action": "SentenceRecognition"}
	body, _, err := s.do(ctx, "POST", s.endpoint, "application/json", payload, headers)
	if err != nil {
		return Result{}, err
	}
	response := object(body["Response"])
	if e := object(response["Error"]); len(e) > 0 {
		return Result{}, s.failure(200, scalar(e["Code"]), scalar(e["Message"]), scalar(response["RequestId"]), mustJSON(body))
	}
	return Result{Text: scalar(response["Result"]), DurationMS: number(response["AudioDuration"]), RequestID: scalar(response["RequestId"])}, nil
}
func xfyunSignature(secret string, params url.Values) string {
	h := hmac.New(sha1.New, []byte(secret))
	h.Write([]byte(params.Encode()))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
func (s *service) xfyun(ctx context.Context, a Audio) (Result, error) {
	random := rand.Text()[:16]
	params := url.Values{"appId": {s.options.AppID}, "accessKeyId": {s.options.APIKey}, "dateTime": {time.Now().Format("2006-01-02T15:04:05-0700")}, "signatureRandom": {random}, "fileSize": {strconv.Itoa(len(a.Data))}, "fileName": {a.Name}, "durationCheckDisable": {"true"}, "language": {"autodialect"}}
	if s.options.Language != "" {
		params.Set("language", s.options.Language)
	}
	body, _, err := s.do(ctx, "POST", s.endpoint+"/v2/upload?"+params.Encode(), "application/octet-stream", a.Data, map[string]string{"signature": xfyunSignature(s.options.SecretKey, params)})
	if err != nil {
		return Result{}, err
	}
	if code := scalar(body["code"]); code != "000000" && code != "0" {
		return Result{}, s.failure(200, code, scalar(body["descInfo"]), "", mustJSON(body))
	}
	order := scalar(at(body, "content", "orderId"))
	if order == "" {
		return Result{}, s.failure(200, "missing_order", "iFLYTEK did not return an order ID", "", mustJSON(body))
	}
	for {
		if err := wait(ctx, 2*time.Second); err != nil {
			return Result{}, s.failure(0, "pending_task", fmt.Sprintf("Recognition is still pending or was interrupted; order ID: %s; %v", order, err), order, nil)
		}
		query := url.Values{"accessKeyId": {s.options.APIKey}, "dateTime": {time.Now().Format("2006-01-02T15:04:05-0700")}, "signatureRandom": {random}, "orderId": {order}, "resultType": {"transfer"}}
		body, _, err = s.do(ctx, "POST", s.endpoint+"/v2/getResult?"+query.Encode(), "application/json", []byte("{}"), map[string]string{"signature": xfyunSignature(s.options.SecretKey, query)})
		if err != nil {
			return Result{}, err
		}
		if code := scalar(body["code"]); code != "000000" && code != "0" {
			return Result{}, s.failure(200, code, scalar(body["descInfo"]), order, mustJSON(body))
		}
		status := scalar(at(body, "content", "orderInfo", "status"))
		switch status {
		case "0", "3":
			continue
		case "4":
			text, err := xfyunText(scalar(at(body, "content", "orderResult")))
			if err != nil {
				return Result{}, s.failure(200, "invalid_transcript", err.Error(), order, nil)
			}
			return Result{Text: text, DurationMS: number(at(body, "content", "orderInfo", "originalDuration")), RequestID: order}, nil
		case "-1":
			return Result{}, s.failure(200, "task_failed", "iFLYTEK transcription failed; failType="+scalar(at(body, "content", "orderInfo", "failType")), order, mustJSON(body))
		}
		return Result{}, s.failure(200, "unknown_task_status", "Unexpected iFLYTEK task status "+status, order, mustJSON(body))
	}
}
func xfyunText(raw string) (string, error) {
	var result struct {
		Lattice []struct {
			Best string `json:"json_1best"`
		} `json:"lattice"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, lattice := range result.Lattice {
		var best struct {
			ST struct {
				RT []struct {
					WS []struct {
						CW []struct {
							W string `json:"w"`
						} `json:"cw"`
					} `json:"ws"`
				} `json:"rt"`
			} `json:"st"`
		}
		if err := json.Unmarshal([]byte(lattice.Best), &best); err != nil {
			return "", err
		}
		for _, rt := range best.ST.RT {
			for _, ws := range rt.WS {
				if len(ws.CW) > 0 {
					text.WriteString(ws.CW[0].W)
				}
			}
		}
	}
	return text.String(), nil
}
