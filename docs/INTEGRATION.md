# Integrating Skein

Skein is a single static binary with one stable contract: a JSON manifest goes in, a JSON report comes out. There is no daemon, port or database. It runs on Linux, macOS and Windows, amd64 and arm64.

## 1. Install

```sh
curl -fsSL https://raw.githubusercontent.com/Balanov/skein/main/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/Balanov/skein/main/install.ps1 | iex
```

Both scripts download the release binary for the current OS/architecture and verify it against `SHA256SUMS` before installing. Set `SKEIN_INSTALL_DIR` to change the target directory. Checksums prove integrity, not authorship — releases are not signed.

## 2. CLI contract (any language)

```sh
skein run --state .skein --include-tasks manifest.json   # report on stdout
skein stop --state .skein --reason "budget exceeded"     # from any process
skein status --state .skein                              # {"stopped":bool,"epoch":n}
skein reset --state .skein                               # after inspection
```

Exit codes: `0` success, `3` a task failed, `4` the run was stopped, `1` operational error, `2` usage error.

Per run, `<state-dir>/runs/<run-id>/` contains `report.json`, `events.jsonl` (one small JSON event per line: sequence, time, kind, epoch, task, pass, status) and `data/` with the payload files. Workload stdout never mixes into the control stream — harnesses can tail `events.jsonl` for live progress.

Concurrency, blast radius and budget are bounded in the manifest itself: `workers`, `limits.max_tasks`, `limits.max_passes` and per-block `timeout_ms`.

## 3. Go API

```go
import skein "github.com/Balanov/skein"

report, err := skein.Run(ctx, manifest, skein.Options{
    StateDir: ".skein",   // epoch, stop latch, locks, artifacts
    Workers:  8,
})
```

The Go module exposes `Run`, `LoadManifest`, `Validate`, `Stop`, `Reset` and `Status` with the same semantics as the CLI — see the package doc and `skein_test.go`. A runnable example is in `examples/embed-go/`.

## 4. Servers and CI

Skein needs no always-on service: invoke it per job from CI, a task runner, or a systemd unit. On servers, pin a version (`SKEIN_VERSION=0.2.0`) and keep the state directory on local disk — the stop latch and epoch must be visible to every process that can stop the run.
