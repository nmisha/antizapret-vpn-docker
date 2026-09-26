package main

import (
	"errors"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func reviewFilter(t *testing.T) *RegexFilter {
	t.Helper()
	for _, tool := range []string{"grep", "sed", "gawk"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("requires %s", tool)
		}
	}
	path := filepath.Join(t.TempDir(), "exclude")
	if err := os.WriteFile(path, []byte("^excluded$\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rf, err := NewRegexFilter(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rf.cmd.Process.Kill(); _ = rf.stdin.Close(); _ = rf.cmd.Wait() })
	return rf
}

func TestRegexFilterLargeBatch(t *testing.T) {
	rf := reviewFilter(t)
	lines := make([]string, 1001)
	for i := range lines {
		lines[i] = strings.Repeat("a", 2000) + ".example.org"
	}
	got, err := rf.Filter(append(lines, "excluded"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(lines) {
		t.Fatalf("got %d lines, want %d", len(got), len(lines))
	}
	for i := range got {
		if got[i] != lines[i] {
			t.Fatalf("line %d corrupted", i)
		}
	}
	got, err = rf.Filter([]string{"next.example", "excluded"})
	if err != nil || len(got) != 1 || got[0] != "next.example" {
		t.Fatalf("next batch: %v, %v", got, err)
	}
}

func TestRegexFilterUnexpectedEOF(t *testing.T) {
	rf := reviewFilter(t)
	_ = rf.cmd.Process.Kill()
	_, err := rf.Filter([]string{"example.org"})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want unexpected EOF", err)
	}
}

func TestRegexFilterReadErrorUnblocksWriter(t *testing.T) {
	rf := reviewFilter(t)
	rf.scanner.Buffer(make([]byte, 16), 32)
	_, err := rf.Filter([]string{strings.Repeat("a", 2*1024*1024)})
	if err == nil {
		t.Fatal("expected scanner error")
	}
}

type gatedFilterInput struct {
	io.WriteCloser
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (w *gatedFilterInput) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.WriteCloser.Write(p)
}

func TestAdaptListWithPendingFilterUpdate(t *testing.T) {
	dist, custom := reviewFilter(t), reviewFilter(t)
	oldDist, oldCustom := excludeMatcherDist, excludeMatcherCustom
	excludeMatcherDist, excludeMatcherCustom = dist, custom
	defer func() { excludeMatcherDist, excludeMatcherCustom = oldDist, oldCustom }()
	gate := &gatedFilterInput{WriteCloser: dist.stdin, entered: make(chan struct{}), release: make(chan struct{})}
	dist.stdin = gate
	root := useListRoot(t)
	path := filepath.Join(root, "list")
	if err := os.WriteFile(path, []byte("example.org\n"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/list/?raw=1&filter_dist=1&filter_custom=1&file="+url.QueryEscape(path), nil)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { adaptList(response, request); close(done) }()
	<-gate.entered // The request holds the outer read lock while filtering dist.
	updated := make(chan struct{})
	go func() { excludeMatchersLock.Lock(); excludeMatchersLock.Unlock(); close(updated) }()
	// Wait until a pending writer prevents new readers, then finish dist.
	deadline := time.Now().Add(5 * time.Second)
	for excludeMatchersLock.TryRLock() {
		excludeMatchersLock.RUnlock()
		if time.Now().After(deadline) {
			close(gate.release)
			t.Fatal("update did not acquire a pending write lock")
		}
		time.Sleep(time.Millisecond)
	}
	close(gate.release)
	<-done
	<-updated
	if response.Body.String() != "example.org\n" {
		t.Fatalf("unexpected response %q", response.Body.String())
	}
}
