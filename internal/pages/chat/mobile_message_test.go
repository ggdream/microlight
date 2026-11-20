package chat

import "testing"

func TestUserMessageKeepsLineBreaksAndCode(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"first\nsecond", "first  \nsecond"},
		{"中文\n第二行", "中文  \n第二行"},
		{"first  \nsecond", "first  \nsecond"},
		{"```go\na()\nb()\n```", "```go\na()\nb()\n```"},
		{"> first\n> second", "> first  \n> second"},
	} {
		if got := userMessageMarkdown(test.source); got != test.want {
			t.Errorf("%q = %q, want %q", test.source, got, test.want)
		}
	}
}
