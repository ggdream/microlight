package localtools

import (
	search "github.com/ggdream/microlight/pkg/websearch"
	"testing"
)

func TestSearchDefaultPreservesExplicitProvider(t *testing.T) {
	previous, _ := searchRegion.Load().(string)
	t.Cleanup(func() { SetSearchRegion(previous) })
	for _, region := range []string{"en-CN", "en-US"} {
		SetSearchRegion(region)
		if got := (SearchConfig{}).Selected(); got != search.DefaultForRegion(region) {
			t.Fatalf("region=%s default=%s", region, got)
		}
		for _, id := range []search.ID{search.DuckDuckGo, search.BingCN, search.DeepSeek} {
			config := SearchConfig{Provider: id}
			if config.Selected() != id || config.Enabled {
				t.Fatal("default must not override selection or enable search")
			}
		}
	}
}
