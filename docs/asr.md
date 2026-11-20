# Built-in speech recognition

ASR is opt-in (disabled in a fresh install). Settings → Tools keeps a separate
credential/model/endpoint draft for each provider, like web search. All saved
ASR options and secrets participate in the versioned `.settings` export/import;
old exports without ASR leave it disabled. Model-provider changes preserve the
shared ASR configuration. No microphone capture or new native module is added.

`pkg/asr` owns the provider interface, service definitions, HTTP protocols,
format/size checks, WAV metadata, signing, transcripts and provider failures.
`internal/localtools` adapts it to Eino's `transcribe_audio`. The tool accepts
exactly one local path or current-message attachment index, resolves it through
the existing workspace policy, snapshots the bytes and binds their hash to the
approval. Uploads require approval in Ask/Smart modes. Full Access can run them
without a second prompt. Approval identifies both the file and service endpoint.
The existing HTTP transport/proxy is passed to every provider, OAuth exchange
and polling request. Redirects are not followed with credentials. Cancellation
stops requests/polling; already submitted cloud tasks may continue remotely.

## Implemented API variants

The provider name alone does not imply every one of that vendor's ASR products.
These are the implemented **file input** APIs. Provider-side duration checks
remain authoritative for compressed files. WAV duration can be checked locally.
Audio is never silently transcoded. Uploaded chat attachments retain the
application's existing 8 MiB limit; local files use the selected API's limit,
with an application ceiling of 100 MiB.

| Provider | API variant | Configuration / limits |
| --- | --- | --- |
| [DeepSeek](https://api-docs.deepseek.com/guides/asr/) | `/v1/tools/asr`, multipart | API Key; PCM16 WAV/Ogg Opus, 8 MiB, 120 seconds; automatic language |
| [Alibaba Cloud](https://help.aliyun.com/zh/model-studio/qwen-asr-api-reference) | Qwen ASR Chat Completions, inline audio | API Key, model (initially `qwen3-asr-flash`); endpoint override supports workspace/region endpoints; 5-minute short files |
| [Volcengine](https://docs.volcengine.com/docs/doubao-voice/large-model-recording-file-lite-recognition-api?lang=en) | file flash `/api/v3/auc/bigmodel/recognize/flash` | API Key; resource `volc.bigasr.auc_turbo` requires activation, potentially unavailable to new accounts without support; 2 hours / 100 MiB |
| [Tencent Cloud](https://cloud.tencent.com/document/product/1093/35646) | `SentenceRecognition`, TC3 signature | Secret ID/Key; engine in model field; 60 seconds / 3 MB |
| [Baidu](https://ai.baidu.com/ai-doc/SPEECH/Jlbxdezuf) | `/server_api`, OAuth client credentials | API/Secret Key; numeric `dev_pid`, raw PCM rate configurable, 60 seconds |
| [iFLYTEK](https://static.xfyun.cn/doc/spark/asr_llm/Ifasr_llm.html) | large-model `/v2/upload` → `/v2/getResult` | App ID/API Key/Secret; HMAC-SHA1 signature; language defaults `autodialect`; 5 hours, application caps files at 100 MiB |
| [Huawei SIS](https://support.huaweicloud.com/api-sis/sis_03_0094.html) | `/v1/{project}/asr/short-audio` | temporary IAM Token, Project ID, regional endpoint, `property` in model field; 60 seconds / base64 payload ≤4 MB |
| [Xiaomi MiMo](https://mimo.mi.com/docs/zh-CN/api/audio/Speech-Recognition) | Chat Completions `mimo-v2.5-asr` | API Key; WAV/MP3, base64 ≤10 MB |
| [Zhipu](https://docs.bigmodel.cn/api-reference/模型-api/语音转文本) | `/audio/transcriptions`, `glm-asr-2512` | API Key; 30 seconds / 25 MiB |
| [OpenAI](https://developers.openai.com/api/docs/guides/speech-to-text) | `/audio/transcriptions` | API Key, configurable model; JSON transcript |
| [Google Cloud](https://docs.cloud.google.com/speech-to-text/docs/v1/authentication) | v1 `speech:recognize` | temporary OAuth 2 access token, language; 60 seconds / 10 MB; self-describing WAV/FLAC or explicit compressed encoding/rate |
| [Azure Speech](https://learn.microsoft.com/rest/api/speechtotext/transcriptions/transcribe?view=rest-speechtotext-2025-10-15) | fast transcription, API 2025-10-15 | subscription key, **required resource endpoint**, optional locale |

The adapters use HTTP APIs in Go, so native, mobile and JS/Wasm builds share the
same implementation without pulling in twelve SDKs. Browser requests still
require vendor CORS support; they cannot bypass it. IAM/OAuth token refresh for
Huawei/Google is manual and the UI calls this out. The UI exposes endpoint editing only for Alibaba Cloud workspace/region URLs, Huawei regional URLs and Azure resource URLs. Fixed-endpoint providers hide this field; saved endpoint overrides remain readable. The Base URL setting is a
request endpoint override for direct-upload APIs; iFLYTEK, Huawei and Azure
expect their service root as described above.

## Failures and validation

HTTP failures and provider errors inside HTTP 200 responses are returned as
structured tool results. Status, error code, original provider diagnostic and
request/order ID are retained, including unrecognized codes. Credential values
and temporary Baidu OAuth tokens are redacted. Successful responses without
text are reported as an empty-transcript error, never fabricated success.
No content-specific ASR rule is added to the general system prompt.

Local checks cover all twelve request/response formats, error propagation,
credentials, file limits, cancellation, no credential-bearing redirects,
approval blocking, attachment selection and settings migration. On 2026-11-20,
DeepSeek and MiMo were also checked against their real APIs with a short speech
sample using existing local credentials. Both returned the expected transcript.
Other providers have protocol fixture coverage, not a claim of live credential
verification.
