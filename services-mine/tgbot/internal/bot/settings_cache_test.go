package bot

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSettingsCacheOwnsSlices(t *testing.T) {
	useTestSettings(t)
	s := getSettingsCached()
	s.IgnoredJoinIDs = []int64{123, 456}
	setSettingsCached(s)
	s.IgnoredJoinIDs[0] = 789
	got := getSettingsCached()
	if got.IgnoredJoinIDs[0] != 123 {
		t.Fatal("cache aliases published settings")
	}
	got.IgnoredJoinIDs[0] = 789
	if getSettingsCached().IgnoredJoinIDs[0] != 123 {
		t.Fatal("cache aliases returned settings")
	}
}

func TestSettingsFailedSavePreservesCache(t *testing.T) {
	useTestSettings(t)
	if err := setJoinIgnored(123, true); err != nil {
		t.Fatal(err)
	}
	if err := setJoinIgnored(456, true); err != nil {
		t.Fatal(err)
	}
	// A non-directory parent fails Save even when the tests run as root.
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	gSettings.Path = filepath.Join(parent, "settings.json")
	if err := setJoinIgnored(123, false); err == nil {
		t.Fatal("save succeeded unexpectedly")
	}
	if !isJoinIgnored(123) || !isJoinIgnored(456) {
		t.Fatal("failed save changed the cache")
	}
}

func TestSettingsIgnoreReadDuringRemoval(t *testing.T) {
	useTestSettings(t)
	for _, id := range []int64{123, 456} {
		if err := setJoinIgnored(id, true); err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = isJoinIgnored(456)
			}
		}
	}()
	for i := 0; i < 100; i++ {
		if err := setJoinIgnored(123, false); err != nil {
			t.Error(err)
			break
		}
		if err := setJoinIgnored(123, true); err != nil {
			t.Error(err)
			break
		}
	}
	close(stop)
	wg.Wait()
}
