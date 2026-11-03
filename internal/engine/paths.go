package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

func dataName(taskID string) string {
	sum := sha256.Sum256([]byte(taskID))
	return hex.EncodeToString(sum[:16])
}

func OutputPath(stateDir, taskID string) string {
	return filepath.Join(stateDir, "data", dataName(taskID)+".out")
}

func passOutputPath(stateDir, taskID string, pass int) string {
	return filepath.Join(stateDir, "data", fmt.Sprintf("%s.pass-%d.out", dataName(taskID), pass))
}

func ErrorPath(stateDir, taskID string, pass int) string {
	return filepath.Join(stateDir, "data", fmt.Sprintf("%s.pass-%d.err", dataName(taskID), pass))
}

func inputPath(stateDir, taskID string) string {
	return filepath.Join(stateDir, "data", dataName(taskID)+".input")
}
