package chat

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var clarificationTool = &schema.ToolInfo{
	Name: timeline.AskUserTool,
	Desc: "Ask the user one focused clarification when a missing detail would materially change the answer. Otherwise answer directly. Supply up to five concise options only if meaningful alternatives exist. The user can always type their own answer. Call this tool at most once per response.",
	ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"question": {Type: schema.String, Required: true, Desc: "One direct question in the user's language. Do not append directions such as 'please choose', 'select 1 or 2', or a list of options."},
		"options":  {Type: schema.Array, ElemInfo: &schema.ParameterInfo{Type: schema.String}, Desc: "Optional suggestions in the user's language. Use only the answer text, with no numbering, letters, or bullets. Omit for free-text input."},
	}),
}

var (
	choiceInstruction = regexp.MustCompile(`(?i)(?:^|[?？。！!：:;；]\s*|\s+)(?:请\s*)?(?:选(?:择)?\s*(?:(?:以下|下列)(?:的)?(?:一个)?选项|(?:其中)?(?:一|任一|1)项|(?:其中)?一个|其一|[0-9０-９A-D]+(?:\s*(?:或(?:者)?|和|、|/|／|与)\s*[0-9０-９A-D]+)+)?|(?:please\s+)?(?:choose|select)\s+(?:one(?:\s+of\s+the\s+(?:following\s+)?options)?|[0-9A-D]+\s*(?:or|/)\s*[0-9A-D]+))\s*[:：。.!！?？]?$`)
	optionPrefix      = regexp.MustCompile(`^(?:(?:[0-9０-９]+|[A-Da-d])[.．]([^0-9０-９]|$)|(?:[0-9０-９]+|[A-Da-d])[、)）]|[(（](?:[0-9０-９]+|[A-Da-d])[)）]|[①②③④⑤⑥⑦⑧⑨⑩]|[-*•][ \t]+)`)
)

func clarificationQuestionText(question string) string {
	question = strings.TrimSpace(question)
	if match := choiceInstruction.FindStringIndex(question); match != nil {
		end := match[0]
		switch {
		case strings.HasPrefix(question[end:], "？"):
			end += len("？")
		case strings.HasPrefix(question[end:], "?"):
			end++
		}
		question = strings.TrimSpace(question[:end])
	}
	if question == "" {
		return locales.FallbackQuestion()
	}
	return question
}

func clarificationOptionText(option string) string {
	return strings.TrimSpace(optionPrefix.ReplaceAllString(strings.TrimSpace(option), "$1"))
}

type clarification struct {
	Question       string
	Options        []string
	Answer         string
	InterruptID    string
	CheckpointID   string
	Model          modelChoice          `json:",omitzero"`
	ActivityOffset *int                 `json:",omitempty"`
	Approval       *localtools.Approval `json:",omitempty"`

	answerOpen bool
}

type askUserTool struct{}

func (askUserTool) Info(context.Context) (*schema.ToolInfo, error) {
	return clarificationTool, nil
}

func (askUserTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	if target, hasAnswer, answer := tool.GetResumeContext[string](ctx); target && hasAnswer {
		return strings.TrimSpace(answer), nil
	}
	question, err := parseClarification(args)
	if err != nil {
		return "", err
	}
	info, err := json.Marshal(question)
	if err != nil {
		return "", err
	}
	return "", tool.Interrupt(ctx, string(info))
}

func parseClarification(args string) (clarification, error) {
	var fields struct {
		Question string
		Options  []string
	}
	if err := json.Unmarshal([]byte(args), &fields); err != nil || strings.TrimSpace(fields.Question) == "" {
		return clarification{}, errors.New(locales.InvalidClarification())
	}
	question := clarification{Question: clarificationQuestionText(fields.Question)}
	for _, option := range fields.Options {
		option = clarificationOptionText(option)
		if option != "" && !slices.Contains(question.Options, option) && len(question.Options) < 5 {
			question.Options = append(question.Options, option)
		}
	}
	return question, nil
}
