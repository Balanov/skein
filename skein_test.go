package skein_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	skein "github.com/Balanov/skein"
)

// TestHelperProcess is not a real test: it is the workload executable for the
// embedding tests below. The test binary re-runs itself with this flag.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("SKEIN_WANT_HELPER") != "1" {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(append([]byte("ok:"), data...)); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func helperManifest() skein.Manifest {
	return skein.Manifest{
		Version: 1,
		Workers: 2,
		Blocks: map[string]skein.Block{
			"append": {
				Argv:      []string{os.Args[0], "-test.run=TestHelperProcess"},
				Env:       []string{"SKEIN_WANT_HELPER=1"},
				TimeoutMS: 10000,
			},
		},
		Tasks: []skein.Task{
			{ID: "a", Block: "append", Input: "start", Passes: 2},
			{ID: "b", Block: "append", Needs: []string{"a"}, StdinFrom: "a"},
		},
	}
}

func TestRunMultiPassDataFlow(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	report, err := skein.Run(context.Background(), helperManifest(), skein.Options{
		StateDir:     stateDir,
		Workers:      2,
		IncludeTasks: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != skein.Succeeded || report.Succeeded != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	output, err := os.ReadFile(report.Tasks["b"].OutputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(output) != "ok:ok:ok:start" {
		t.Fatalf("unexpected output %q", output)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "runs")); err != nil {
		t.Fatalf("run artifacts missing: %v", err)
	}
}

func TestStopLatchAndReset(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := skein.Stop(stateDir, "test stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	stopped, epoch, err := skein.State(stateDir)
	if err != nil || !stopped || epoch != 2 {
		t.Fatalf("Status = %v, %d, %v", stopped, epoch, err)
	}
	if _, err := skein.Run(context.Background(), helperManifest(), skein.Options{StateDir: stateDir}); !errors.Is(err, skein.ErrStopped) {
		t.Fatalf("Run err = %v, want ErrStopped", err)
	}
	if err := skein.Reset(stateDir, false); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	stopped, epoch, err = skein.State(stateDir)
	if err != nil || stopped || epoch != 2 {
		t.Fatalf("after reset: %v, %d, %v", stopped, epoch, err)
	}
}
