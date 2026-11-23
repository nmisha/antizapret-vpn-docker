package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type BotLogger struct {
	Path string
	mu   sync.Mutex
}

func NewBotLogger(usersFilePath string) *BotLogger {
	dir := filepath.Dir(usersFilePath)
	return &BotLogger{Path: filepath.Join(dir, "bot.log")}
}

func (bl *BotLogger) Append(line string) {
	if bl == nil {
		return
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()

	f, err := os.OpenFile(bl.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.WriteString(line)
	if !strings.HasSuffix(line, "\n") {
		_, _ = f.WriteString("\n")
	}
}

func (bl *BotLogger) Clear() error {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	return os.WriteFile(bl.Path, []byte{}, 0644)
}

func (bl *BotLogger) ReadAll(maxBytes int) (string, error) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	b, err := os.ReadFile(bl.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if maxBytes > 0 && len(b) > maxBytes {
		// keep tail
		b = b[len(b)-maxBytes:]
	}
	return string(b), nil
}

func (bl *BotLogger) ReadTailLines(n int, maxBytes int) (string, error) {
	if n <= 0 {
		n = 80
	}
	// reuse ReadAll with maxBytes to avoid reading huge files
	text, err := bl.ReadAll(maxBytes)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// remove trailing empty line
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n"), nil
	}
	return strings.Join(lines[len(lines)-n:], "\n"), nil
}

func (bl *BotLogger) LogIn(user User, chatID int64, text string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = truncate(text, 900)
	bl.Append(fmt.Sprintf("%s IN  user=%s tg_id=%d chat=%d text=%s", ts, user.Name, user.TelegramID, chatID, text))
}

func (bl *BotLogger) LogOut(user User, chatID int64, text string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = truncate(text, 900)
	bl.Append(fmt.Sprintf("%s OUT user=%s tg_id=%d chat=%d text=%s", ts, user.Name, user.TelegramID, chatID, text))
}
