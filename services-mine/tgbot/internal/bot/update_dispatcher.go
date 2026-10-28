package bot

import (
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// updateDispatcher handles updates of one sender strictly in order (a button sets a pending
// input, the next message fills it) and updates of different senders in parallel, so a slow
// backend call (wg-easy, telemt) holds up only the person who made it.
//
// Each sender gets a queue and a goroutine on the first update; the goroutine exits after
// idleTimeout without updates. At most maxParallel handlers run at once.
// maxSenders bounds all queues and goroutines, including workers waiting for a slot.
type updateDispatcher struct {
	handle      func(tgbotapi.Update)
	sem         chan struct{}
	queueLen    int
	maxSenders  int
	idleTimeout time.Duration

	mu     sync.Mutex
	queues map[int64]chan tgbotapi.Update
}

const (
	updateMaxParallel = 16
	updateMaxSenders  = 2048 // ~4 KB queue + goroutine each; excess new senders are dropped until an idle queue exits
	updateQueueLen    = 32   // per sender; more is a flood and is dropped
	updateIdleTimeout = time.Minute
)

func newUpdateDispatcher(maxParallel, queueLen int, idleTimeout time.Duration, handle func(tgbotapi.Update)) *updateDispatcher {
	return &updateDispatcher{
		handle:      handle,
		sem:         make(chan struct{}, maxParallel),
		queueLen:    queueLen,
		maxSenders:  updateMaxSenders,
		idleTimeout: idleTimeout,
		queues:      map[int64]chan tgbotapi.Update{},
	}
}

// updateSenderID is the ordering key: messages and button presses of one person share it.
func updateSenderID(u tgbotapi.Update) int64 {
	switch {
	case u.Message != nil && u.Message.From != nil:
		return u.Message.From.ID
	case u.CallbackQuery != nil && u.CallbackQuery.From != nil:
		return u.CallbackQuery.From.ID
	}
	return 0 // updates without a sender share one queue
}

func (d *updateDispatcher) submit(u tgbotapi.Update) {
	key := updateSenderID(u)
	d.mu.Lock()
	defer d.mu.Unlock()
	q, ok := d.queues[key]
	if !ok {
		if len(d.queues) >= d.maxSenders {
			log.Printf("update sender limit reached, update %d from %d dropped", u.UpdateID, key)
			return
		}
		q = make(chan tgbotapi.Update, d.queueLen)
		d.queues[key] = q
		go d.run(key, q)
	}
	select {
	case q <- u:
	default:
		log.Printf("update queue of sender %d is full, update %d dropped", key, u.UpdateID)
	}
}

func (d *updateDispatcher) run(key int64, q chan tgbotapi.Update) {
	idle := time.NewTimer(d.idleTimeout)
	defer idle.Stop()
	for {
		select {
		case u := <-q:
			d.sem <- struct{}{}
			d.safeHandle(u)
			<-d.sem
			idle.Reset(d.idleTimeout)
		case <-idle.C:
			// submit enqueues under d.mu, so an empty queue checked under d.mu stays empty.
			d.mu.Lock()
			if len(q) > 0 {
				d.mu.Unlock()
				idle.Reset(d.idleTimeout)
				continue
			}
			delete(d.queues, key)
			d.mu.Unlock()
			return
		}
	}
}

// safeHandle keeps a panic in one handler from taking the whole bot down.
func (d *updateDispatcher) safeHandle(u tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			msg := "panic while handling update"
			log.Printf("%s %d from %d: %v\n%s", msg, u.UpdateID, updateSenderID(u), r, debug.Stack())
			if gLogger != nil && getSettingsCached().LogErrorsEnabled {
				gLogger.Append(formatLogLine("PANIC", 0, "", truncate(msg+": "+fmt.Sprint(r), 900)))
			}
		}
	}()
	d.handle(u)
}
