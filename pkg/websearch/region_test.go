package websearch

import "testing"

func TestDefaultSearchUsesExplicitRegion(t *testing.T) {
	for _, tc := range []struct {
		locale string
		want   ID
	}{
		{"zh-CN", BingCN}, {"en-CN", BingCN}, {"zh-Hans-CN", BingCN}, {"zh_CN.UTF-8", BingCN},
		{"en-US", DuckDuckGo}, {"zh-TW", DuckDuckGo}, {"zh-HK", DuckDuckGo}, {"zh-MO", DuckDuckGo},
		{"zh-SG", DuckDuckGo}, {"zh-Hans", DuckDuckGo}, {"zh", DuckDuckGo}, {"", DuckDuckGo}, {"invalid_locale", DuckDuckGo},
	} {
		if got := DefaultForRegion(tc.locale); got != tc.want {
			t.Errorf("%q: got %s want %s", tc.locale, got, tc.want)
		}
	}
}
