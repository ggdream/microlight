package chat

import (
	"context"
	"io"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type usageMetrics struct {
	Requests       int           `json:"requests,omitempty"`
	UsageSamples   int           `json:"usage_samples,omitempty"`
	Input          int           `json:"input,omitempty"`
	Output         int           `json:"output,omitempty"`
	Cached         int           `json:"cached,omitempty"`
	Reasoning      int           `json:"reasoning,omitempty"`
	ModelTime      time.Duration `json:"model_time,omitempty"`
	FirstTokenTime time.Duration `json:"first_token_time,omitempty"`
	FirstTokens    int           `json:"first_tokens,omitempty"`
	DecodeTime     time.Duration `json:"decode_time,omitempty"`
	DecodedTokens  int           `json:"decoded_tokens,omitempty"`
}

func (m *usageMetrics) add(n usageMetrics) {
	m.Requests += n.Requests
	m.UsageSamples += n.UsageSamples
	m.Input += n.Input
	m.Output += n.Output
	m.Cached += n.Cached
	m.Reasoning += n.Reasoning
	m.ModelTime += n.ModelTime
	m.FirstTokenTime += n.FirstTokenTime
	m.FirstTokens += n.FirstTokens
	m.DecodeTime += n.DecodeTime
	m.DecodedTokens += n.DecodedTokens
}

func (chat *conversation) metrics() (total usageMetrics) {
	for _, entry := range chat.Turns {
		total.add(entry.Metrics)
	}
	return total
}

type metricsReporterKey struct{}

// meteredModel observes each model request below the agent loop, including tool
// continuations and context summaries. The reporter belongs to a single run.
type meteredModel struct{ model.AgenticModel }

func (m meteredModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	report, _ := ctx.Value(metricsReporterKey{}).(func(usageMetrics))
	start := time.Now()
	msg, err := m.AgenticModel.Generate(ctx, input, opts...)
	if report != nil && err == nil {
		sample := modelSample(time.Since(start), modelUsage(msg))
		report(sample)
	}
	return msg, err
}

func (m meteredModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	report, _ := ctx.Value(metricsReporterKey{}).(func(usageMetrics))
	start := time.Now()
	stream, err := m.AgenticModel.Stream(ctx, input, opts...)
	if err != nil || report == nil {
		return stream, err
	}
	var first, last time.Time
	var usage *schema.TokenUsage
	return schema.StreamReaderWithConvert(stream, func(chunk *schema.AgenticMessage) (*schema.AgenticMessage, error) {
		if generatesTokens(chunk) {
			last = time.Now()
			if first.IsZero() {
				first = last
			}
		}
		// Usage chunks contain cumulative totals for this request.
		if snapshot := modelUsage(chunk); snapshot != nil {
			usage = snapshot
		}
		return chunk, nil
	}, schema.WithOnEOF(func() (any, error) {
		sample := modelSample(time.Since(start), usage)
		if !first.IsZero() {
			sample.FirstTokenTime = first.Sub(start)
			sample.FirstTokens = 1
			if last.After(first) && sample.Output > 0 {
				sample.DecodeTime = last.Sub(first)
				sample.DecodedTokens = sample.Output
			}
		}
		report(sample)
		return nil, io.EOF
	})), nil
}

func modelUsage(msg *schema.AgenticMessage) *schema.TokenUsage {
	if msg == nil || msg.ResponseMeta == nil {
		return nil
	}
	return msg.ResponseMeta.TokenUsage
}

func modelSample(duration time.Duration, usage *schema.TokenUsage) usageMetrics {
	sample := usageMetrics{Requests: 1, ModelTime: duration}
	if usage != nil && (usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		sample.UsageSamples = 1
		sample.Input = max(0, usage.PromptTokens)
		sample.Output = max(0, usage.CompletionTokens)
		sample.Cached = min(sample.Input, max(0, usage.PromptTokenDetails.CachedTokens))
		sample.Reasoning = min(sample.Output, max(0, usage.CompletionTokensDetails.ReasoningTokens))
	}
	return sample
}

func generatesTokens(msg *schema.AgenticMessage) bool {
	if msg == nil {
		return false
	}
	for _, block := range msg.ContentBlocks {
		if block != nil && (block.AssistantGenText != nil && block.AssistantGenText.Text != "" ||
			block.Reasoning != nil && block.Reasoning.Text != "" ||
			block.FunctionToolCall != nil && block.FunctionToolCall.Arguments != "") {
			return true
		}
	}
	return false
}
