package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type QueryLogEntry struct {
	T      string `json:"T"`
	QH     string `json:"QH"`
	QT     string `json:"QT"`
	IP     string `json:"IP"`
	Result struct {
		Rules []struct {
			Text string `json:"Text"`
		} `json:"Rules"`
	} `json:"Result"`
}

type QueryLogReadResult struct {
	NewOffset      int64
	FileSize       int64
	FileModTime    time.Time
	ResetToStart   bool
	UsedOffsetRead bool
	LinesRead      int
}

func processNewEntries(path string, cursor CursorState, handler func(QueryLogEntry) error) (QueryLogReadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return QueryLogReadResult{}, fmt.Errorf("open querylog: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return QueryLogReadResult{}, fmt.Errorf("stat querylog: %w", err)
	}

	offset := cursor.Offset
	reset := shouldResetCursor(info, cursor)
	if info.Size() < offset || reset {
		offset = 0
		reset = true
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return QueryLogReadResult{}, fmt.Errorf("seek querylog: %w", err)
	}

	linesRead := 0
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var e QueryLogEntry
			if jsonErr := json.Unmarshal(line, &e); jsonErr == nil {
				linesRead++
				if handler != nil {
					if handleErr := handler(e); handleErr != nil {
						return QueryLogReadResult{}, handleErr
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return QueryLogReadResult{}, fmt.Errorf("read querylog: %w", err)
		}
	}

	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return QueryLogReadResult{}, fmt.Errorf("get querylog offset: %w", err)
	}
	return QueryLogReadResult{
		NewOffset:      newOffset,
		FileSize:       info.Size(),
		FileModTime:    info.ModTime().UTC(),
		ResetToStart:   reset,
		UsedOffsetRead: offset > 0 && !reset,
		LinesRead:      linesRead,
	}, nil
}

func shouldResetCursor(info os.FileInfo, cursor CursorState) bool {
	if cursor.Offset == 0 {
		return false
	}
	if cursor.FileSize > 0 && info.Size() < cursor.FileSize {
		return true
	}
	if cursor.FileModTime == "" {
		return false
	}
	prevMod, err := time.Parse(time.RFC3339Nano, cursor.FileModTime)
	if err != nil {
		return false
	}
	// If the file timestamp moved backwards or changed while size also shrank,
	// treat it as log rewrite/truncate and reread from start.
	if info.ModTime().UTC().Before(prevMod) {
		return true
	}
	return false
}
