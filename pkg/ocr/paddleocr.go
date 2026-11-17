package ocr

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	paddle "github.com/PaddlePaddle/PaddleOCR/api_sdk/go"
)

type Model string

const (
	PPV6        Model = paddle.PPOCRv6
	PPV5        Model = paddle.PPOCRv5
	PPV5Latin   Model = paddle.PPOCRv5Latin
	VL16        Model = paddle.PaddleOCRVL16
	StructureV3 Model = paddle.PPStructureV3
)

func TextModels() []Model     { return []Model{PPV6, PPV5, PPV5Latin} }
func DocumentModels() []Model { return []Model{VL16, StructureV3} }

// Provider analyzes a staged image or PDF.
type Provider interface {
	Recognize(ctx context.Context, model Model, path string) (string, error)
	ParseDocument(ctx context.Context, model Model, path string) (string, error)
}

type PaddleOCR struct {
	client *paddle.Client
}

func NewPaddleOCR(token string, transport http.RoundTripper) (*PaddleOCR, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}
	client, err := paddle.NewClient(
		paddle.WithToken(token),
		paddle.WithBaseURL(paddle.DefaultBaseURL),
		paddle.WithHTTPClient(&http.Client{
			Transport: paddleTransport{transport},
			Timeout:   60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 || req.URL.Scheme != "https" || via[0].Method != http.MethodGet {
					return http.ErrUseLastResponse
				}
				if req.URL.Host != via[0].URL.Host {
					req.Header.Del("Authorization")
				}
				return nil
			},
		}),
	)
	if err != nil {
		return nil, err
	}
	return &PaddleOCR{client: client}, nil
}

func (p *PaddleOCR) Recognize(ctx context.Context, model Model, path string) (string, error) {
	if !slices.Contains(TextModels(), model) {
		return "", errors.New("unsupported PaddleOCR text recognition model")
	}
	result, err := p.client.OCR(ctx, &paddle.OCRRequest{
		Model: string(model), FilePath: path,
		Options: &paddle.OCROptions{Visualize: paddle.Bool(false)},
	})
	if err != nil {
		return "", err
	}
	var lines []string
	for _, page := range result.Pages {
		pruned, _ := page.PrunedResult.(map[string]any)
		texts, ok := pruned["rec_texts"].([]any)
		if !ok {
			return "", errors.New("PaddleOCR response is missing recognized text")
		}
		for _, text := range texts {
			line, _ := text.(string)
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (p *PaddleOCR) ParseDocument(ctx context.Context, model Model, path string) (string, error) {
	var options paddle.DocParsingOptionsProvider
	switch model {
	case VL16:
		options = &paddle.PaddleOCRVLOptions{Visualize: paddle.Bool(false), ReturnMarkdownImages: paddle.Bool(false)}
	case StructureV3:
		options = &paddle.PPStructureV3Options{Visualize: paddle.Bool(false), ReturnMarkdownImages: paddle.Bool(false)}
	}
	if options == nil {
		return "", errors.New("unsupported PaddleOCR document parsing model")
	}
	result, err := p.client.ParseDocument(ctx, &paddle.DocParsingRequest{
		Model: string(model), FilePath: path, Options: options,
	})
	if err != nil {
		return "", err
	}
	var pages []string
	for _, page := range result.Pages {
		if markdown := strings.TrimSpace(page.MarkdownText); markdown != "" {
			pages = append(pages, markdown)
		}
	}
	return strings.Join(pages, "\n\n"), nil
}
