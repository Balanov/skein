package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResetRecoversEpochAfterInterruptedStop(t *testing.T) {
	state := t.TempDir()
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StopPath(state), []byte(`{"epoch":42,"time":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, epochFileName), []byte("41"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Reset(state, false); err != nil {
		t.Fatal(err)
	}
	epoch, err := CurrentEpoch(state)
	if err != nil || epoch != 42 {
		t.Fatalf("epoch=%d err=%v", epoch, err)
	}
}
