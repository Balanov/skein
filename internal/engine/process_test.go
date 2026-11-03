package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("SKEIN_TEST_HELPER") != "1" {
		return
	}
	if os.Getenv("SKEIN_TEST_ROLE") == "child" {
		heartbeat := os.Getenv("SKEIN_TEST_HEARTBEAT")
		for counter := 0; ; counter++ {
			_ = os.WriteFile(heartbeat, []byte(strconv.Itoa(counter)), 0o600)
			time.Sleep(10 * time.Millisecond)
		}
	}
	child := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	child.Env = append(os.Environ(), "SKEIN_TEST_ROLE=child")
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_ = child.Wait()
	os.Exit(0)
}

func TestEmergencyStopKillsProcessTree(t *testing.T) {
	if testing.Short() {
		t.Skip("process-tree test in short mode")
	}
	state := t.TempDir()
	heartbeat := filepath.Join(t.TempDir(), "heartbeat.txt")
	manifest := Manifest{Version: 1, Workers: 1, Blocks: map[string]Block{
		"tree": {
			Argv:      []string{os.Args[0], "-test.run=TestHelperProcess"},
			Env:       []string{"SKEIN_TEST_HELPER=1", "SKEIN_TEST_HEARTBEAT=" + heartbeat},
			TimeoutMS: 30000,
		},
	}, Tasks: []Task{{ID: "tree", Block: "tree"}, {ID: "queued", Block: "tree"}}}
	done := make(chan Report, 1)
	errDone := make(chan error, 1)
	go func() {
		report, err := Run(context.Background(), manifest, Options{StateDir: state, PollInterval: time.Millisecond})
		done <- report
		errDone <- err
	}()
	waitFor(t, 5*time.Second, func() bool {
		data, err := os.ReadFile(heartbeat)
		return err == nil && len(data) > 0
	})
	if err := Stop(state, "process tree test"); err != nil {
		t.Fatal(err)
	}
	select {
	case report := <-done:
		if report.Status != Stopped || report.Succeeded != 0 {
			t.Fatalf("unexpected process report: %+v", report)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("run did not stop")
	}
	if err := <-errDone; err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	after, err := os.ReadFile(heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("child process survived stop: heartbeat %q -> %q", before, after)
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
