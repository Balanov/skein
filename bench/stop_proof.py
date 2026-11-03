#!/usr/bin/env python3
"""Stop real parent/child trees and verify that every child heartbeat freezes."""
import argparse
import json
import os
import platform
import statistics
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path


CHILD_CODE = """from pathlib import Path
import os, sys, time
path = Path(sys.argv[1])
temp = path.with_suffix('.tmp')
counter = 0
while True:
    temp.write_text(f'{os.getpid()}:{counter}', encoding='utf-8')
    os.replace(temp, path)
    counter += 1
    time.sleep(0.005)
"""
PARENT_CODE = """from pathlib import Path
import subprocess, sys
Path(sys.argv[1]).write_text('parent-ready', encoding='utf-8')
child = subprocess.Popen([sys.executable, '-c', sys.argv[3], sys.argv[2]])
child.wait()
"""


def run_checked(command, **kwargs):
    return subprocess.run(command, check=True, capture_output=True, text=True, encoding="utf-8", **kwargs)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--skein", required=True)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--runs", type=int, default=20)
    parser.add_argument("--warmups", type=int, default=3)
    parser.add_argument("--tasks", type=int, default=8)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if args.runs < 1 or args.warmups < 0 or args.tasks < 1:
        parser.error("runs/tasks must be positive and warmups cannot be negative")
    samples = []
    for iteration in range(args.warmups + args.runs):
        with tempfile.TemporaryDirectory(prefix="skein-stop-") as directory:
            root = Path(directory)
            state = root / "state"
            ready = root / "ready"
            ready.mkdir()
            tasks = []
            blocks = {}
            heartbeats = []
            for index in range(args.tasks):
                parent_marker = ready / f"{index}.parent"
                heartbeat = ready / f"{index}.heartbeat"
                heartbeats.append(heartbeat)
                name = f"tree-{index}"
                blocks[name] = {"argv": [args.python, "-c", PARENT_CODE, str(parent_marker), str(heartbeat), CHILD_CODE], "timeout_ms": 35000}
                tasks.append({"id": name, "block": name})
            manifest = {"version": 1, "workers": args.tasks, "blocks": blocks, "tasks": tasks}
            manifest_path = root / "manifest.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            process = subprocess.Popen([args.skein, "run", "--state", str(state), "--no-events", str(manifest_path)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8")
            try:
                deadline = time.monotonic() + 20
                while len(list(ready.glob("*.parent"))) < args.tasks or len(list(ready.glob("*.heartbeat"))) < args.tasks:
                    if process.poll() is not None or time.monotonic() > deadline:
                        stdout, stderr = process.communicate(timeout=2)
                        raise RuntimeError(f"parent/child trees did not start: {stdout}\n{stderr}")
                    time.sleep(0.005)
                started = time.perf_counter_ns()
                run_checked([args.skein, "stop", "--state", str(state), "--reason", "benchmark"])
                stdout, stderr = process.communicate(timeout=15)
                elapsed = time.perf_counter_ns() - started
                report = json.loads(stdout)
                if process.returncode != 4 or report["status"] != "stopped" or report["succeeded"] != 0 or report["stopped"] != args.tasks:
                    raise RuntimeError(f"unexpected stop result: code={process.returncode}, report={report}, stderr={stderr}")

                before = [path.read_text(encoding="utf-8") for path in heartbeats]
                time.sleep(0.150)
                after = [path.read_text(encoding="utf-8") for path in heartbeats]
                frozen = all(value and value == after[index] for index, value in enumerate(before))
                if not frozen:
                    raise RuntimeError(f"at least one child heartbeat survived: {before} -> {after}")

                stopped_status = json.loads(run_checked([args.skein, "status", "--state", str(state)]).stdout)
                run_checked([args.skein, "stop", "--state", str(state), "--reason", "duplicate must not advance"])
                duplicate_status = json.loads(run_checked([args.skein, "status", "--state", str(state)]).stdout)
                run_checked([args.skein, "reset", "--state", str(state)])
                reset_status = json.loads(run_checked([args.skein, "status", "--state", str(state)]).stdout)
                latch_ok = stopped_status == {"stopped": True, "epoch": 2} and duplicate_status == stopped_status and reset_status == {"stopped": False, "epoch": 2}
                if not latch_ok:
                    raise RuntimeError(f"stop/reset contract failed: {stopped_status}, {duplicate_status}, {reset_status}")
                if iteration >= args.warmups:
                    samples.append({
                        "run": iteration - args.warmups + 1,
                        "stop_to_all_exited_ns": elapsed,
                        "return_code": process.returncode,
                        "stopped": report["stopped"],
                        "parent_child_trees": args.tasks,
                        "child_heartbeats_frozen": frozen,
                        "duplicate_stop_preserved_epoch": True,
                        "reset_preserved_epoch": True,
                    })
            finally:
                if process.poll() is None:
                    subprocess.run([args.skein, "stop", "--state", str(state), "--reason", "proof cleanup"], capture_output=True)
                    try:
                        process.communicate(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.communicate()
    durations = sorted(sample["stop_to_all_exited_ns"] for sample in samples)
    result = {
        "schema": 2,
        "measured_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Wall time from starting the stop CLI until the run process exits after terminating tracked real parent/child Python process trees. Includes stop-process launch, latch propagation, process-tree termination and report output. Heartbeat, duplicate-stop and reset checks happen after the measured interval.",
        "method": f"{args.warmups} unrecorded warmups, then {args.runs} measured runs. Every run starts {args.tasks} parents and {args.tasks} children. Every child updates an atomic heartbeat file; all heartbeats must remain unchanged for 150 ms after the run exits. p95 uses nearest rank.",
        "machine": {"platform": platform.platform(), "machine": platform.machine(), "processor": platform.processor(), "logical_cpus": os.cpu_count(), "python": platform.python_version()},
        "warmup_runs": args.warmups,
        "tasks_per_run": args.tasks,
        "processes_per_run": args.tasks * 2,
        "samples": samples,
        "summary": {"runs": len(samples), "median_ms": statistics.median(durations) / 1e6, "p95_ms": durations[max(0, (len(durations) * 95 + 99) // 100 - 1)] / 1e6, "max_ms": durations[-1] / 1e6, "trees_stopped": sum(sample["parent_child_trees"] for sample in samples), "child_heartbeats_verified_frozen": sum(1 for sample in samples if sample["child_heartbeats_frozen"]) * args.tasks},
    }
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(result["summary"], indent=2))


if __name__ == "__main__":
    main()
