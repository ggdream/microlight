package a2uiagent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ggdream/microlight/pkg/a2ui"
	"github.com/ggdream/microlight/pkg/a2uiagent"
)

func TestOfficialMultiCatalogInference(t *testing.T) {
	for _, tc := range fixtures(t, "multi_catalog_formats") {
		if tc.Action != "compile" && tc.Action != "parse_response" && tc.Action != "create_format" && tc.Action != "decompile" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			if strings.Contains(tc.Name, "below_v1_0") || strings.Contains(tc.Name, "v0_9") {
				t.Skip("legacy protocol")
			}
			if tc.Action == "create_format" {
				var catalogs []*a2ui.Catalog
				for _, value := range tc.Args.Catalogs {
					catalogs = append(catalogs, catalogFor(t, value.(map[string]any)["catalog"]))
				}
				if _, err := (a2uiagent.Config{Catalogs: catalogs}).NewStream(tc.Args.Format, nil, nil); err == nil {
					t.Fatal("expected duplicate catalog error")
				}
				return
			}
			catalogs := fixtureCatalogs(t, tc)
			if tc.Action == "decompile" {
				checkMultiCatalogDecompiler(t, tc, catalogs)
				return
			}
			var got any
			var err error
			if tc.Action == "parse_response" {
				parts, parseErr := a2uiagent.ParseResponse(tc.Args.Format, tc.Input, catalogs, nil, false)
				err = parseErr
				wire := make([]map[string]any, 0, len(parts))
				for _, part := range parts {
					if part.Text != "" {
						wire = append(wire, map[string]any{"text": part.Text})
					}
					if len(part.Messages) != 0 {
						wire = append(wire, map[string]any{"a2ui": part.Messages})
					}
				}
				got = wire
			} else if tc.Args.Format == a2uiagent.Express {
				got, err = a2uiagent.CompileExpressCatalogs(tc.Input, catalogs, nil)
			} else {
				got, err = a2uiagent.CompileDirectJSON(tc.Input, catalogs)
			}
			if tc.ExpectError != nil {
				if err == nil {
					t.Fatal("expected inference rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(got, tc.Expect) {
				actual, _ := json.Marshal(got)
				want, _ := json.Marshal(tc.Expect)
				t.Fatalf("got %s, want %s", actual, want)
			}
		})
	}
}

func checkMultiCatalogDecompiler(t *testing.T, tc officialCase, catalogs []*a2ui.Catalog) {
	t.Helper()
	raw, err := json.Marshal(tc.Messages)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := a2ui.DecodeBatch(raw)
	if err != nil {
		t.Fatal(err)
	}
	source, err := a2uiagent.Decompile(tc.Args.Format, messages, catalogs...)
	if tc.ExpectError != nil {
		if err == nil {
			t.Fatal("expected decompiler error")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range tc.ExpectContains {
		if !strings.Contains(source, fragment) && !a2uiagent.ContainsExpressFragment(source, fragment) {
			t.Errorf("missing %q in %s", fragment, source)
		}
	}
	for _, fragment := range tc.ExpectAbsent {
		if strings.Contains(source, fragment) {
			t.Errorf("unexpected %q in %s", fragment, source)
		}
	}
	if !tc.ExpectRoundTrip {
		return
	}
	var result []a2ui.AgentMessage
	if tc.Args.Format == a2uiagent.Express {
		result, err = a2uiagent.CompileExpressCatalogs(source, catalogs, nil)
	} else {
		result, err = a2uiagent.CompileDirectJSON(source, catalogs)
	}
	if err != nil || !jsonEqual(result, messages) {
		t.Fatalf("round trip changed messages: %s (%v)", source, err)
	}
}

func TestOfficialExpressDecompiler(t *testing.T) {
	for _, tc := range fixtures(t, "express_decompiler") {
		t.Run(tc.Name, func(t *testing.T) {
			if strings.Contains(tc.Name, "before_v1_0") || strings.Contains(tc.Name, "v0_9") {
				t.Skip("legacy protocol")
			}
			tc.Args.Format = a2uiagent.Express
			checkMultiCatalogDecompiler(t, tc, fixtureCatalogs(t, tc))
		})
	}
}
