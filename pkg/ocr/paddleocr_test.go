package ocr

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestPaddleOCRUploadsAndReadsResults(t *testing.T) {
	for _, model := range TextModels() {
		t.Run(string(model), func(t *testing.T) {
			checkPaddleOCR(t, model, "capture.png",
				`{"result":{"ocrResults":[{"prunedResult":{"rec_texts":["微光助手","","Microlight"]}}]}}`,
				"微光助手\nMicrolight")
		})
	}
	for _, model := range DocumentModels() {
		t.Run(string(model), func(t *testing.T) {
			checkPaddleOCR(t, model, "scan.pdf",
				`{"result":{"layoutParsingResults":[{"markdown":{"text":"# Title"}}]}}`+"\n"+
					`{"result":{"layoutParsingResults":[{"markdown":{"text":"| Column |"}}]}}`,
				"# Title\n\n| Column |")
		})
	}
}

func checkPaddleOCR(t *testing.T, model Model, filename, response, want string) {
	t.Helper()

	t.Setenv("PADDLEOCR_BASE_URL", "https://unexpected.example")
	input := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(input, []byte("test input bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	p, err := NewPaddleOCR("test-token", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			defer req.Body.Close()
		}
		calls++
		var body string
		if req.URL.Host == "paddleocr.aistudio-app.com" {
			if req.Header.Get("Authorization") != "Bearer test-token" {
				t.Fatal("missing SDK authorization")
			}
			switch req.Method {
			case http.MethodPost:
				if err := req.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				file, header, err := req.FormFile("file")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil || string(data) != "test input bytes" ||
					header.Filename != filename || req.FormValue("model") != string(model) ||
					!strings.Contains(req.FormValue("optionalPayload"), `"visualize":false`) {
					t.Fatal("unexpected OCR upload")
				}
				body = `{"code":0,"data":{"jobId":"job-test"}}`
			case http.MethodGet:
				if !strings.HasSuffix(req.URL.Path, "/job-test") {
					t.Fatalf("unexpected poll path: %s", req.URL.Path)
				}
				body = `{"code":0,"data":{"jobId":"job-test","state":"done","resultUrl":{"jsonUrl":"https://result.example/ocr.jsonl"}}}`
			}
		} else if req.URL.Host == "result.example" && req.Method == http.MethodGet {
			if req.Header.Get("Authorization") != "" {
				t.Fatal("OCR credentials sent to the result host")
			}
			body = response
		}
		if body == "" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	run := p.Recognize
	if filepath.Ext(input) == ".pdf" {
		run = p.ParseDocument
	}
	got, err := run(t.Context(), model, input)
	if err != nil || got != want || calls != 3 {
		t.Fatalf("got %q, %v, %d requests", got, err, calls)
	}
}
