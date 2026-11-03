package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Balanov/skein/internal/engine"
)

const version = "0.2.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println("skein", version)
		return 0
	}
	switch args[0] {
	case "run":
		return runSwarm(args[1:])
	case "stop":
		return stopSwarm(args[1:])
	case "reset":
		return resetSwarm(args[1:])
	case "status":
		return statusSwarm(args[1:])
	case "help", "--help", "-h":
		usage()
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", args[0])
		usage()
		return 2
	}
}

func runSwarm(args []string) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	state := flags.String("state", ".skein", "swarm control directory")
	workers := flags.Int("workers", 0, "maximum concurrent tasks")
	syncEvents := flags.Bool("sync", false, "sync every audit event to disk")
	includeTasks := flags.Bool("include-tasks", false, "include every task in report.json and stdout")
	noEvents := flags.Bool("no-events", false, "disable the JSONL audit stream")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: skein run [--state DIR] [--workers N] MANIFEST.json")
		return 2
	}
	manifest, err := engine.LoadManifest(flags.Arg(0))
	if err != nil {
		return fail(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	report, err := engine.Run(ctx, manifest, engine.Options{StateDir: *state, Workers: *workers, Events: !*noEvents, SyncEvents: *syncEvents, IncludeTasks: *includeTasks})
	if errors.Is(err, engine.ErrStopped) {
		fmt.Fprintln(os.Stderr, "swarm is stopped; inspect it, then run skein reset before a new run")
		return 4
	}
	if err != nil {
		return fail(err)
	}
	if !*includeTasks {
		report.Tasks = nil
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return fail(err)
	}
	switch report.Status {
	case engine.Stopped:
		return 4
	case engine.Failed:
		return 3
	default:
		return 0
	}
}

func stopSwarm(args []string) int {
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	state := flags.String("state", ".skein", "swarm control directory")
	reason := flags.String("reason", "manual emergency stop", "reason stored in the latch")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		return 2
	}
	if err := engine.Stop(*state, *reason); err != nil {
		return fail(err)
	}
	epoch, err := engine.CurrentEpoch(*state)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("stopped; epoch %d is fenced\n", epoch)
	return 0
}

func resetSwarm(args []string) int {
	flags := flag.NewFlagSet("reset", flag.ContinueOnError)
	state := flags.String("state", ".skein", "swarm control directory")
	force := flags.Bool("force", false, "remove a stale run lock after verifying no run is active")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		return 2
	}
	if err := engine.Reset(*state, *force); err != nil {
		return fail(err)
	}
	epoch, err := engine.CurrentEpoch(*state)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("reset; epoch remains %d\n", epoch)
	return 0
}

func statusSwarm(args []string) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	state := flags.String("state", ".skein", "swarm control directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	epoch, err := engine.CurrentEpoch(*state)
	if err != nil {
		return fail(err)
	}
	status := struct {
		Stopped bool   `json:"stopped"`
		Epoch   uint64 `json:"epoch"`
	}{Stopped: engine.StopRequested(*state), Epoch: epoch}
	if err := json.NewEncoder(os.Stdout).Encode(status); err != nil {
		return fail(err)
	}
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "skein:", err)
	return 1
}

func usage() {
	fmt.Fprintf(os.Stderr, `Skein %s — fail-closed swarm orchestration

Usage:
  skein run [--state DIR] [--workers N] [--sync] MANIFEST.json
  skein stop [--state DIR] [--reason TEXT]
  skein reset [--state DIR] [--force]
  skein status [--state DIR]
  skein --version

A stop advances the epoch and fences queued work and late completions.
Reset never reuses an old epoch.
`, version)
}
