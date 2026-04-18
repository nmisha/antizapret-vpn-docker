package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

func readNewEntries(path string, offset int64) ([]QueryLogEntry, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, fmt.Errorf("open querylog: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, offset, fmt.Errorf("stat querylog: %w", err)
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek querylog: %w", err)
	}

	entries := make([]QueryLogEntry, 0, 32)
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var e QueryLogEntry
			if jsonErr := json.Unmarshal(line, &e); jsonErr == nil {
				entries = append(entries, e)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, offset, fmt.Errorf("read querylog: %w", err)
		}
	}

	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, offset, fmt.Errorf("get querylog offset: %w", err)
	}
	return entries, newOffset, nil
}
