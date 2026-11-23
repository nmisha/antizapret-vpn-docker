package bot

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func msgFrom(sender int64, updateID int) tgbotapi.Update {
	return tgbotapi.Update{UpdateID: updateID, Message: &tgbotapi.Message{From: &tgbotapi.User{ID: sender}}}
}

func cbFrom(sender int64, updateID int) tgbotapi.Update {
	return tgbotapi.Update{UpdateID: updateID, CallbackQuery: &tgbotapi.CallbackQuery{From: &tgbotapi.User{ID: sender}}}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDispatcherKeepsOrderPerSender(t *testing.T) {
	var mu sync.Mutex
	got := map[int64][]int{}
	d := newUpdateDispatcher(4, 64, time.Minute, func(u tgbotapi.Update) {
		time.Sleep(time.Duration(u.UpdateID%3) * time.Millisecond) // uneven handler time
		mu.Lock()
		got[updateSenderID(u)] = append(got[updateSenderID(u)], u.UpdateID)
		mu.Unlock()
	})

	// messages and button presses of one sender interleave and must keep their order
	for i := 0; i < 30; i++ {
		for _, s := range []int64{1, 2, 3} {
			if i%2 == 0 {
				d.submit(msgFrom(s, i))
			} else {
				d.submit(cbFrom(s, i))
			}
		}
	}
	waitFor(t, "all updates", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got[1]) == 30 && len(got[2]) == 30 && len(got[3]) == 30
	})
	mu.Lock()
	defer mu.Unlock()
	for s, ids := range got {
		for i, id := range ids {
			if id != i {
				t.Fatalf("sender %d: order %v", s, ids)
			}
		}
	}
}

func TestDispatcherSlowSenderDoesNotBlockOthers(t *testing.T) {
	release := make(chan struct{})
	fastDone := make(chan struct{})
	d := newUpdateDispatcher(4, 8, time.Minute, func(u tgbotapi.Update) {
		if updateSenderID(u) == 1 {
			<-release // a backend that hangs
			return
		}
		close(fastDone)
	})
	d.submit(msgFrom(1, 1))
	d.submit(msgFrom(2, 2))
	select {
	case <-fastDone:
	case <-time.After(5 * time.Second):
		t.Fatal("sender 2 waited for the slow sender 1")
	}
	close(release)
}

func TestDispatcherLimitsParallelHandlers(t *testing.T) {
	const limit = 3
	var running, peak atomic.Int32
	var done atomic.Int32
	d := newUpdateDispatcher(limit, 8, time.Minute, func(u tgbotapi.Update) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		running.Add(-1)
		done.Add(1)
	})
	for s := int64(1); s <= 12; s++ {
		d.submit(msgFrom(s, int(s)))
	}
	waitFor(t, "12 handlers", func() bool { return done.Load() == 12 })
	if p := peak.Load(); p > limit {
		t.Fatalf("peak %d handlers, limit %d", p, limit)
	}
}

func TestDispatcherIdleSenderGoroutineExits(t *testing.T) {
	var handled atomic.Int32
	d := newUpdateDispatcher(2, 8, 20*time.Millisecond, func(tgbotapi.Update) { handled.Add(1) })
	d.submit(msgFrom(7, 1))
	waitFor(t, "queue removal", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.queues) == 0
	})
	// the sender comes back later and is served by a new goroutine
	d.submit(msgFrom(7, 2))
	waitFor(t, "second update", func() bool { return handled.Load() == 2 })
}

func TestDispatcherDropsFloodAndSurvivesPanic(t *testing.T) {
	block := make(chan struct{})
	var handled atomic.Int32
	d := newUpdateDispatcher(2, 2, time.Minute, func(u tgbotapi.Update) {
		if u.UpdateID == 1 {
			<-block
			panic("handler bug")
		}
		handled.Add(1)
	})
	d.submit(msgFrom(5, 1)) // taken by the goroutine, blocks
	waitFor(t, "first update taken", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.queues[5]) == 0
	})
	for i := 2; i <= 10; i++ { // queue holds 2, the rest is dropped
		d.submit(msgFrom(5, i))
	}
	close(block)
	waitFor(t, "queued updates after the panic", func() bool { return handled.Load() == 2 })
	time.Sleep(20 * time.Millisecond)
	if n := handled.Load(); n != 2 {
		t.Fatalf("handled %d, want 2 (flood dropped)", n)
	}
}

func TestUpdateSettingsKeepsConcurrentChanges(t *testing.T) {
	useTestSettings(t)
	var wg sync.WaitGroup
	for i := int64(1); i <= 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			if err := setJoinIgnored(id, true); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if n := len(getSettingsCached().IgnoredJoinIDs); n != 50 {
		t.Fatalf("ignore list has %d ids, want 50", n)
	}
	s, err := gSettings.Load()
	if err != nil || len(s.IgnoredJoinIDs) != 50 {
		t.Fatalf("saved %d ids, %v", len(s.IgnoredJoinIDs), err)
	}
}

// Real handlers from many senders at once, for the race detector (go test -race).
func TestConcurrentHandlersRace(t *testing.T) {
	useTestSettings(t)
	bot, _ := newFakeTelegram(t)

	users := "["
	for i := 1; i <= 20; i++ {
		if i > 1 {
			users += ","
		}
		users += fmt.Sprintf(`{"telegram_id":%d,"name":"u%d","roles":["Admin"]}`, 1000+i, i)
	}
	us := newTestUsersStore(t, users+"]")

	router := NewRouter()
	reg := NewCommandRegistry(router)
	RegisterAdminHandlers(reg)
	RegisterUsersAdminHandlers(reg)
	h := &updateHandler{bot: bot, usersStore: us, store: NewStore(filepath.Join(t.TempDir(), "domains.txt")), router: router}

	var wg sync.WaitGroup
	d := newUpdateDispatcher(8, 16, time.Minute, func(u tgbotapi.Update) {
		defer wg.Done()
		h.handleUpdate(u)
	})
	for i := int64(1); i <= 20; i++ {
		id := 1000 + i
		chat := &tgbotapi.Chat{ID: id, Type: "private"}
		for j, text := range []string{"/roles", "/users", "/log_tail 50"} {
			wg.Add(1)
			d.submit(tgbotapi.Update{UpdateID: j, Message: &tgbotapi.Message{From: &tgbotapi.User{ID: id}, Chat: chat, Text: text}})
		}
		wg.Add(2)
		d.submit(tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "q", From: &tgbotapi.User{ID: id}, Data: usrCb("u", idStr(id)),
			Message: &tgbotapi.Message{MessageID: 1, Chat: chat}}})
		d.submit(tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "q", From: &tgbotapi.User{ID: id}, Data: usrCb("rt", idStr(id), "Support"),
			Message: &tgbotapi.Message{MessageID: 1, Chat: chat}}})
	}
	wg.Wait()

	// every concurrent role toggle landed in users.json
	list, err := us.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range list {
		if !u.HasExact(RoleSupport) {
			t.Fatalf("%s lost its Support role", u.Name)
		}
	}
}

func TestDispatcherBoundsAllSenders(t *testing.T) {
	release := make(chan struct{}, 5)
	var handled atomic.Int32
	d := newUpdateDispatcher(1, 8, 20*time.Millisecond, func(tgbotapi.Update) {
		<-release
		handled.Add(1)
	})
	d.maxSenders = 3
	t.Cleanup(func() { close(release) })
	for id := int64(1); id <= 100; id++ {
		d.submit(msgFrom(id, int(id)))
	}
	d.mu.Lock()
	count := len(d.queues)
	d.mu.Unlock()
	if count != 3 {
		t.Fatalf("%d queues, want 3", count)
	}
	// Existing senders remain admitted at the global limit.
	d.submit(cbFrom(1, 101))
	release <- struct{}{}
	release <- struct{}{}
	release <- struct{}{}
	release <- struct{}{}
	waitFor(t, "admitted updates", func() bool { return handled.Load() == 4 })
	waitFor(t, "idle queues exit", func() bool { d.mu.Lock(); defer d.mu.Unlock(); return len(d.queues) == 0 })
	d.submit(msgFrom(100, 102))
	release <- struct{}{}
	waitFor(t, "new sender after capacity freed", func() bool { return handled.Load() == 5 })
}
