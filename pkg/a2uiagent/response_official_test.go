package a2uiagent_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2uiagent"
	"gopkg.in/yaml.v3"
)

func TestOfficialResponseParser(t *testing.T) {
	for _, format := range []a2uiagent.Format{a2uiagent.DirectJSON, a2uiagent.Express} {
		raw, err := os.ReadFile("testdata/official/" + string(format) + "_response_parser.yaml")
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Name   string
			Action string
			Input  string
			Args   struct {
				Complete bool
				Wrapped  *bool
				Catalog  any
				Catalogs []any
			}
			Parts           []map[string]any
			Expect          any
			ExpectOutput    *string  `yaml:"expect_output"`
			ExpectContains  []string `yaml:"expect_contains"`
			ExpectRoundTrip bool     `yaml:"expect_round_trip"`
			ExpectError     any      `yaml:"expect_error"`
		}
		if err := yaml.Unmarshal(raw, &cases); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			t.Run(tc.Name, func(t *testing.T) {
				switch tc.Action {
				case "unwrap":
					actual := responseFields(a2uiagent.UnwrapResponse(format, tc.Input))
					if !jsonEqual(actual, tc.Expect) {
						t.Fatalf("unwrapped %#v, want %#v", actual, tc.Expect)
					}
				case "wrap":
					input := make([]a2uiagent.ResponsePart, 0, len(tc.Parts))
					for _, fields := range tc.Parts {
						var part a2uiagent.ResponsePart
						part.Text, _ = fields["text"].(string)
						if payload, ok := fields["a2ui_raw"].(string); ok {
							part.Payload, part.Final = &payload, true
						}
						input = append(input, part)
					}
					output := a2uiagent.WrapResponse(format, input)
					if tc.ExpectOutput != nil && output != *tc.ExpectOutput {
						t.Fatalf("wrapped %q, want %q", output, *tc.ExpectOutput)
					}
					for _, fragment := range tc.ExpectContains {
						if !strings.Contains(output, fragment) {
							t.Errorf("missing %q in %q", fragment, output)
						}
					}
					if tc.ExpectRoundTrip && !jsonEqual(responseFields(a2uiagent.UnwrapResponse(format, output)), tc.Parts) {
						t.Fatalf("wrap/unwrap changed parts: %s", output)
					}
				case "has_format_content":
					if got := a2uiagent.HasFormatContent(format, tc.Input, tc.Args.Complete); got != tc.Expect {
						t.Fatalf("has format content = %v, want %v", got, tc.Expect)
					}
				case "parse_response":
					var fixture officialCase
					fixture.Args.Catalog, fixture.Args.Catalogs = tc.Args.Catalog, tc.Args.Catalogs
					parts, err := a2uiagent.ParseResponse(format, tc.Input, fixtureCatalogs(t, fixture), nil, tc.Args.Wrapped != nil && !*tc.Args.Wrapped)
					if (err != nil) != (tc.ExpectError != nil) {
						t.Fatalf("parse error = %v, expected %v", err, tc.ExpectError)
					}
					if err == nil {
						actual := make([]map[string]any, 0, len(parts))
						for _, part := range parts {
							if part.Messages != nil {
								actual = append(actual, map[string]any{"a2ui": part.Messages})
							} else {
								actual = append(actual, map[string]any{"text": part.Text})
							}
						}
						if !jsonEqual(actual, tc.Expect) {
							t.Fatalf("parsed %#v, want %#v", actual, tc.Expect)
						}
					}
				default:
					t.Fatalf("unhandled response action %q", tc.Action)
				}
			})
		}
	}
}

func responseFields(parts []a2uiagent.ResponsePart) []map[string]any {
	result := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		if part.Payload == nil {
			result = append(result, map[string]any{"text": part.Text})
			continue
		}
		fields := map[string]any{"a2ui_raw": *part.Payload}
		if !part.Final {
			fields["is_final"] = false
		}
		result = append(result, fields)
	}
	return result
}
