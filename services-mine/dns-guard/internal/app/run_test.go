package app

import (
	"testing"
	"time"
)

func TestProfileWhitelistedMatchesByKindAndNameCaseInsensitive(t *testing.T) {
	ref := ProfileRef{Kind: "WG", Name: " Alice-Phone "}
	whitelist := map[string][]string{
		"wg":   {"alice-phone"},
		"ovpn": {"other"},
	}
	if !profileWhitelisted(ref, whitelist) {
		t.Fatal("expected profile to be whitelisted")
	}
}

func TestProfileWhitelistedDoesNotMatchOtherKind(t *testing.T) {
	ref := ProfileRef{Kind: "awg", Name: "alice-phone"}
	whitelist := map[string][]string{
		"wg": {"alice-phone"},
	}
	if profileWhitelisted(ref, whitelist) {
		t.Fatal("did not expect profile to be whitelisted for another kind")
	}
}

func TestInitializeQueryLogCursorEnablesCatchupForExistingAPIState(t *testing.T) {
	cfg := Config{
		QueryLogSource:              "api",
		HistoryCatchupEnabled:       true,
		HistoryCatchupMaxAgeMinutes: 10,
	}
	state := &State{
		Cursor: CursorState{
			LastSeenTime: "2026-04-19T10:00:00Z",
		},
		Profiles: map[string]ProfileRiskState{},
	}
	source := &stubQueryLogSource{}

	catchup, err := initializeQueryLogCursor(cfg, source, state)
	if err != nil {
		t.Fatalf("initialize cursor: %v", err)
	}
	if !catchup {
		t.Fatal("expected catchup to be enabled")
	}
	if source.syncCalls != 0 {
		t.Fatalf("expected no sync call, got %d", source.syncCalls)
	}
	if state.Cursor.LastSeenTime != "2026-04-19T10:00:00Z" {
		t.Fatalf("last seen changed unexpectedly: %q", state.Cursor.LastSeenTime)
	}
}

func TestInitializeQueryLogCursorFallsBackToSyncWhenCatchupDisabled(t *testing.T) {
	cfg := Config{
		QueryLogSource:        "api",
		HistoryCatchupEnabled: false,
	}
	state := &State{Profiles: map[string]ProfileRiskState{}}
	source := &stubQueryLogSource{}

	catchup, err := initializeQueryLogCursor(cfg, source, state)
	if err != nil {
		t.Fatalf("initialize cursor: %v", err)
	}
	if catchup {
		t.Fatal("did not expect catchup")
	}
	if source.syncCalls != 1 {
		t.Fatalf("expected one sync call, got %d", source.syncCalls)
	}
}

type stubQueryLogSource struct {
	syncCalls int
}

func (s *stubQueryLogSource) Sync(cursor *CursorState) error {
	s.syncCalls++
	cursor.LastSeenTime = time.Now().UTC().Format(time.RFC3339Nano)
	return nil
}

func (s *stubQueryLogSource) Process(_ *CursorState, _ QueryLogProcessOptions, _ func(QueryLogRecord) error) error {
	return nil
}

func (s *stubQueryLogSource) Description() string {
	return "stub"
}
