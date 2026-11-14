package localtools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFetchRequiresConfiguredTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("fetch bypassed the configured transport")
	}))
	defer server.Close()
	for _, client := range []*http.Client{nil, {}} {
		if _, err := fetchLocalAgentURL(t.Context(), client, server.URL, 0); err == nil {
			t.Fatal("fetch should require a transport")
		}
	}
}

func TestFetchLargeHTMLThroughRedirect(t *testing.T) {
	content := strings.Repeat("正文。", webChunkRunes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusMovedPermanently)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<title>Flutter Docs</title><script>%s</script><nav>Navigation</nav><main><h1>Development</h1><p>%s</p></main>",
			strings.Repeat("x", maxLocalRead+1), content)
	}))
	defer server.Close()

	var combined strings.Builder
	for offset := 0; ; {
		page := fetchTestPage(t, server.Client(), server.URL+"/old", offset)
		if page.Title != "Flutter Docs" || page.URL != server.URL+"/new" || !utf8.ValidString(page.Content) {
			t.Fatalf("unexpected page at offset %d: title %q, URL %q", offset, page.Title, page.URL)
		}
		if count := utf8.RuneCountInString(page.Content); count > webChunkRunes || (offset == 0 && count != webChunkRunes) {
			t.Fatalf("unexpected character count %d at offset %d", count, offset)
		}
		combined.WriteString(page.Content)
		if page.NextOffset == 0 {
			break
		}
		if page.NextOffset != combined.Len() || page.NextOffset <= offset {
			t.Fatalf("invalid next offset %d after %d", page.NextOffset, offset)
		}
		offset = page.NextOffset
	}
	if combined.String() != "Development\n"+content {
		t.Fatal("extracted chunks do not reproduce the main content")
	}
}

func TestFetchReportsRedirectOutsideApprovedSite(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("fetched a destination on a different port")
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/page", http.StatusFound)
	}))
	defer server.Close()
	page := fetchTestPage(t, server.Client(), server.URL, 0)
	if page.RedirectURL != destination.URL+"/page" || page.Content != "" {
		t.Fatalf("unexpected redirect result: %+v", page)
	}
}

func fetchTestPage(t *testing.T, client *http.Client, target string, offset int) webPage {
	t.Helper()
	result, err := fetchLocalAgentURL(t.Context(), client, target, offset)
	if err != nil {
		t.Fatal(err)
	}
	var page webPage
	if err := json.Unmarshal([]byte(result), &page); err != nil {
		t.Fatal(err)
	}
	return page
}
