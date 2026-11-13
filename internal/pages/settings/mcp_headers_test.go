package settings

import (
	"testing"

	"github.com/ggdream/microlight/internal/mcptools"
)

func TestMCPHeaderPresetsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		header mcptools.Header
		preset mcpHeaderPreset
		value  string
	}{
		{mcptools.Header{Name: "Authorization", Value: "Bearer test-token"}, mcpHeaderBearer, "test-token"},
		{mcptools.Header{Name: "X-API-Key", Value: "test-key"}, mcpHeaderAPIKey, "test-key"},
		{mcptools.Header{Name: "Authorization", Value: "Basic test-value"}, mcpHeaderCustom, "Basic test-value"},
		{mcptools.Header{Name: "X-Custom-Key", Value: "test-key"}, mcpHeaderCustom, "test-key"},
	} {
		draft := editMCPHeader(tc.header)
		if draft.preset != tc.preset || draft.value != tc.value || draft.requestHeader() != tc.header {
			t.Errorf("could not edit and save header %q", tc.header.Name)
		}
	}
	header := (mcpHeaderDraft{preset: mcpHeaderBearer}).requestHeader()
	if err := (mcptools.Server{URL: "https://example.com", Headers: []mcptools.Header{header}}).Validate(); err == nil {
		t.Fatal("an empty bearer token passed validation")
	}
}
