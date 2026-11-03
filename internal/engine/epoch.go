package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const epochFileName = "EPOCH"

func CurrentEpoch(stateDir string) (uint64, error) {
	if record, err := readStop(stateDir); err == nil && record.Epoch > 0 {
		return record.Epoch, nil
	}
	return storedEpoch(stateDir)
}

func storedEpoch(stateDir string) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, epochFileName))
	if errors.Is(err, os.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	epoch, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil || epoch == 0 {
		return 0, errors.New("invalid epoch file")
	}
	return epoch, nil
}

func writeEpoch(stateDir string, epoch uint64) error {
	temp, err := os.CreateTemp(stateDir, ".epoch-*")
	if err != nil {
		return err
	}
	path := temp.Name()
	defer os.Remove(path)
	if _, err := fmt.Fprintf(temp, "%d", epoch); err != nil {
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
	return os.Rename(path, filepath.Join(stateDir, epochFileName))
}

func LeaseValid(lease Lease, currentEpoch uint64) bool {
	return lease.Epoch == currentEpoch && lease.RunID != "" && lease.Sequence != 0 && lease.TaskID != "" && lease.Pass > 0
}

func WriteLease(stateDir string, lease Lease) (string, error) {
	directory := filepath.Join(stateDir, "leases")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, dataName(lease.TaskID)+fmt.Sprintf(".%d.lease", lease.Pass))
	data, err := json.Marshal(lease)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
