package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExclude(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exclude")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newFilter(t *testing.T, contents string) *RegexFilter {
	t.Helper()
	rf, err := NewRegexFilter(writeExclude(t, contents))
	if err != nil {
		t.Fatal(err)
	}
	return rf
}

func useFilters(t *testing.T, dist, custom *RegexFilter) {
	t.Helper()
	oldDist, oldCustom := excludeMatcherDist.Load(), excludeMatcherCustom.Load()
	excludeMatcherDist.Store(dist)
	excludeMatcherCustom.Store(custom)
	t.Cleanup(func() { excludeMatcherDist.Store(oldDist); excludeMatcherCustom.Store(oldCustom) })
}

func TestRegexFilterLargeBatch(t *testing.T) {
	rf := newFilter(t, "^excluded$\n")
	lines := make([]string, 1001)
	for i := range lines {
		lines[i] = strings.Repeat("a", 2000) + ".example.org"
	}
	got := rf.Filter(append(lines, "excluded"))
	if len(got) != len(lines) {
		t.Fatalf("got %d lines, want %d", len(got), len(lines))
	}
	for i := range got {
		if got[i] != lines[i] {
			t.Fatalf("line %d corrupted", i)
		}
	}
}

// The grep-based filter hung forever when a pattern matched its delimiter.
func TestRegexFilterPatternMatchingEverythingUppercase(t *testing.T) {
	rf := newFilter(t, "[A-Z]\n_\n")
	got := rf.Filter([]string{"__DELIM__", "example.org", "Upper.org"})
	if len(got) != 1 || got[0] != "example.org" {
		t.Fatalf("got %q", got)
	}
}

// An empty pattern matches every line, so blank lines must never become one.
func TestRegexFilterIgnoresBlankLinesAndComments(t *testing.T) {
	rf := newFilter(t, "\r\n   \n# comment\nexample\\.com  \r\n\n")
	got := rf.Filter([]string{"example.com", "keep.org", "# comment"})
	if len(got) != 2 || got[0] != "keep.org" || got[1] != "# comment" {
		t.Fatalf("got %q", got)
	}
}

func TestRegexFilterEmptyFileKeepsEverything(t *testing.T) {
	rf := newFilter(t, "\n\n")
	got := rf.Filter([]string{"a.org", "b.org"})
	if len(got) != 2 {
		t.Fatalf("got %q", got)
	}
}

func TestRegexFilterSkipsInvalidPatterns(t *testing.T) {
	rf := newFilter(t, "(unclosed\nbad\\.org\n")
	got := rf.Filter([]string{"bad.org", "(unclosed", "good.org"})
	if len(got) != 2 || got[0] != "(unclosed" || got[1] != "good.org" {
		t.Fatalf("got %q", got)
	}
}

func TestRegexFilterSlashWrappedAndGNUWordBoundary(t *testing.T) {
	rf := newFilter(t, "/foo[0-9]+\\.net/\n\\<vk\\>\n")
	got := rf.Filter([]string{"foo12.net", "vk.com", "vkontakte.ru", "other.net"})
	if len(got) != 2 || got[0] != "vkontakte.ru" || got[1] != "other.net" {
		t.Fatalf("got %q", got)
	}
}

func TestRegexFilterDoesNotModifySourceFile(t *testing.T) {
	contents := "a  \r\n\n# c\n"
	path := writeExclude(t, contents)
	if _, err := NewRegexFilter(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != contents {
		t.Fatalf("file changed: %q, %v", data, err)
	}
}

func TestUpdateRegexFilterKeepsPreviousOnError(t *testing.T) {
	previous := newFilter(t, "old\n")
	useFilters(t, previous, previous)
	oldDistPath, oldCustomPath := excludeDistPath, excludeCustomPath
	t.Cleanup(func() { excludeDistPath, excludeCustomPath = oldDistPath, oldCustomPath })
	excludeDistPath = writeExclude(t, "new\n")
	excludeCustomPath = filepath.Join(t.TempDir(), "missing")

	if err := updateRegexFilter(); err == nil {
		t.Fatal("expected an error for a missing custom file")
	}
	if excludeMatcherDist.Load() != previous || excludeMatcherCustom.Load() != previous {
		t.Fatal("filters were replaced after a failed update")
	}
}

func TestAdaptListFiltersDistAndCustom(t *testing.T) {
	useFilters(t, newFilter(t, "^dist\\.org$\n"), newFilter(t, "custom\n"))
	root := useListRoot(t)
	path := filepath.Join(root, "list")
	if err := os.WriteFile(path, []byte("dist.org\ncustom.org\nexample.org\n"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/list/?raw=1&filter_dist=1&filter_custom=1&file="+url.QueryEscape(path), nil)
	response := httptest.NewRecorder()
	adaptList(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "example.org\n" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

// A missing filter must be an error status, never a 200 with a partial list.
func TestAdaptListFailsBeforeStreamingWithoutFilter(t *testing.T) {
	useFilters(t, nil, nil)
	root := useListRoot(t)
	path := filepath.Join(root, "list")
	if err := os.WriteFile(path, []byte("example.org\n"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/list/?raw=1&filter_custom=1&file="+url.QueryEscape(path), nil)
	response := httptest.NewRecorder()
	adaptList(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}
