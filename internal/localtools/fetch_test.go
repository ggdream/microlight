package localtools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFetchExtractsLargeHTMLAndTruncatesUTF8(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><style>hidden CSS</style><script>hidden JS</script><h1>标题</h1><p>"+strings.Repeat("正文", 15000)+"</p></html>")
	}))
	defer s.Close()
	text, err := fetchLocalAgentURL(context.Background(), s.Client(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "标题") || !strings.Contains(text, "已截断") || strings.Contains(text, "hidden") || !utf8.ValidString(text) || len(text) > 64<<10 {
		t.Fatalf("unexpected bounded readable result: length=%d, prefix=%q", len(text), text[:min(100, len(text))])
	}
}

func TestFetchRedirectPolicyPreservesApprovalBoundary(t *testing.T) {
	otherCalled := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherCalled = true }))
	defer other.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/end", http.StatusFound)
		case "/end":
			fmt.Fprint(w, "arrived")
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusTemporaryRedirect)
		case "/other":
			http.Redirect(w, r, other.URL, http.StatusMovedPermanently)
		}
	}))
	defer s.Close()
	if text, err := fetchLocalAgentURL(context.Background(), s.Client(), s.URL+"/start"); err != nil || text != "arrived" {
		t.Fatalf("same-origin redirect: %q %v", text, err)
	}
	if _, err := fetchLocalAgentURL(context.Background(), s.Client(), s.URL+"/loop"); err == nil || !strings.Contains(err.Error(), "5 次") {
		t.Fatalf("loop was not bounded: %v", err)
	}
	if _, err := fetchLocalAgentURL(context.Background(), s.Client(), s.URL+"/other"); err == nil || !strings.Contains(err.Error(), other.URL) || otherCalled {
		t.Fatalf("redirect bypassed approval: %v, called=%v", err, otherCalled)
	}
}

func TestFetchRejectsBinaryAndHonorsCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte{0, 1, 2}) }))
	defer s.Close()
	if _, err := fetchLocalAgentURL(context.Background(), s.Client(), s.URL); err == nil {
		t.Fatal("accepted binary")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchLocalAgentURL(ctx, s.Client(), s.URL); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestReadableHTMLKeepsResolvedLinks(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<p>Documentation <a href="/guide">Guide</a><a href="javascript:alert(1)">Ignored URL</a></p>`)
	}))
	defer s.Close()
	text, err := fetchLocalAgentURL(context.Background(), s.Client(), s.URL)
	if err != nil || !strings.Contains(text, s.URL+"/guide") || strings.Contains(text, "javascript:") {
		t.Fatalf("links not preserved safely: %q %v", text, err)
	}
}
