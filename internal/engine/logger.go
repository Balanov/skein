package engine

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type eventLogger struct {
	mu     sync.Mutex
	file   *os.File
	buffer *bufio.Writer
	sync   bool
	seq    uint64
	err    error
}

func openEventLogger(path string, syncEvents bool) (*eventLogger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &eventLogger{file: file, buffer: bufio.NewWriterSize(file, 64*1024), sync: syncEvents}, nil
}

func (logger *eventLogger) emit(kind, taskID string, pass int, epoch uint64, status Status, message string) {
	if logger == nil {
		return
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.err != nil {
		return
	}
	logger.seq++
	event := Event{Sequence: logger.seq, Time: time.Now().UTC(), Kind: kind, TaskID: taskID, Pass: pass, Epoch: epoch, Status: status, Error: message}
	if err := json.NewEncoder(logger.buffer).Encode(event); err != nil {
		logger.err = err
		return
	}
	if logger.sync {
		if err := logger.buffer.Flush(); err != nil {
			logger.err = err
			return
		}
		if err := logger.file.Sync(); err != nil {
			logger.err = err
		}
	}
}

func (logger *eventLogger) close() error {
	if logger == nil {
		return nil
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.err == nil {
		logger.err = logger.buffer.Flush()
	}
	if err := logger.file.Close(); logger.err == nil {
		logger.err = err
	}
	return logger.err
}
