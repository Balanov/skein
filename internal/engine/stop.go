package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const StopFileName = "STOP"
const LockFileName = "RUN.lock"
const stoppingFileName = "STOPPING"

type StopRecord struct {
	Epoch  uint64    `json:"epoch"`
	Time   time.Time `json:"time"`
	Reason string    `json:"reason,omitempty"`
}

func StopPath(stateDir string) string {
	return filepath.Join(stateDir, StopFileName)
}

func readStop(stateDir string) (StopRecord, error) {
	var record StopRecord
	data, err := os.ReadFile(StopPath(stateDir))
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("invalid stop record: %w", err)
	}
	return record, nil
}

// Stop is idempotent and preserves the first latch record. Advancing the epoch
// fences queued work and late completions, including work not yet dispatched.
func Stop(stateDir, reason string) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	guard, err := acquireStopGuard(stateDir)
	if err != nil || guard == nil {
		return err
	}
	guardPath := guard.Name()
	guard.Close()
	defer os.Remove(guardPath)

	path := StopPath(stateDir)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	epoch, err := CurrentEpoch(stateDir)
	if err != nil {
		return err
	}
	epoch++
	temp, err := os.CreateTemp(stateDir, ".stop-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	record := StopRecord{Epoch: epoch, Time: time.Now().UTC(), Reason: reason}
	if err := json.NewEncoder(temp).Encode(record); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("latch stop: %w", err)
	}
	return writeEpoch(stateDir, epoch)
}

func acquireStopGuard(stateDir string) (*os.File, error) {
	path := filepath.Join(stateDir, stoppingFileName)
	for attempt := 0; attempt < 100; attempt++ {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return file, nil
		}
		if StopRequested(stateDir) {
			return nil, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil, errors.New("stop is already being written; stale STOPPING may require manual removal")
}

func StopRequested(stateDir string) bool {
	_, err := os.Stat(StopPath(stateDir))
	return err == nil
}

func Reset(stateDir string, force bool) error {
	lockPath := filepath.Join(stateDir, LockFileName)
	if _, err := os.Stat(lockPath); err == nil && !force {
		return errors.New("run lock exists; stop the active run first or use --force after checking it is stale")
	}
	if force {
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if record, err := readStop(stateDir); err == nil {
		current, currentErr := storedEpoch(stateDir)
		if currentErr != nil {
			return currentErr
		}
		if record.Epoch > current {
			if err := writeEpoch(stateDir, record.Epoch); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(StopPath(stateDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
