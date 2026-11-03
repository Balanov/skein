#!/usr/bin/env python3
"""Measure end-to-end stop acknowledgement and process-tree termination."""
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


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--skein", required=True)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--runs", type=int, default=12)
    parser.add_argument("--tasks", type=int, default=8)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    samples = []
    for run in range(args.runs):
        with tempfile.TemporaryDirectory(prefix="skein-stop-") as directory:
            root = Path(directory)
            state = root / "state"
            ready = root / "ready"
            ready.mkdir()
            tasks = []
            for index in range(args.tasks):
                marker = ready / f"{index}.ready"
                code = "from pathlib import Path; import sys,time; Path(sys.argv[1]).write_text('ready'); time.sleep(30)"
                tasks.append({"id": f"sleeper-{index}", "block": "sleep", "input": "x", "passes": 1})
                tasks[-1]["marker"] = str(marker)
            # Markers are embedded into argv without a shell.
            for task in tasks:
                task.pop("input")
                task["argv_marker"] = task.pop("marker")
            manifest = {
                "version": 1,
                "workers": args.tasks,
                "blocks": {"sleep": {"argv": [args.python, "-c", "from pathlib import Path; import sys,time; Path(sys.argv[1]).write_text('ready'); time.sleep(30)", "MARKER"], "timeout_ms": 35000}},
                "tasks": tasks,
            }
            # Each task gets its own block so its marker is explicit.
            manifest["blocks"] = {}
            for index, task in enumerate(tasks):
                marker = task.pop("argv_marker")
                name = f"sleep-{index}"
                task["block"] = name
                manifest["blocks"][name] = {"argv": [args.python, "-c", code, marker], "timeout_ms": 35000}
            manifest_path = root / "manifest.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            process = subprocess.Popen([args.skein, "run", "--state", str(state), "--no-events", str(manifest_path)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8")
            deadline = time.monotonic() + 15
            while len(list(ready.glob("*.ready"))) < args.tasks:
                if process.poll() is not None or time.monotonic() > deadline:
                    stdout, stderr = process.communicate(timeout=2)
                    raise RuntimeError(f"agents did not start: {stdout}\n{stderr}")
                time.sleep(0.005)
            started = time.perf_counter_ns()
            stop = subprocess.run([args.skein, "stop", "--state", str(state), "--reason", "benchmark"], check=True, capture_output=True, text=True)
            stdout, stderr = process.communicate(timeout=15)
            elapsed = time.perf_counter_ns() - started
            report = json.loads(stdout)
            if process.returncode != 4 or report["status"] != "stopped" or report["succeeded"] != 0:
                raise RuntimeError(f"unexpected stop result: code={process.returncode}, report={report}, stderr={stderr}")
            samples.append({"run": run + 1, "stop_to_all_exited_ns": elapsed, "return_code": process.returncode, "stopped": report["stopped"]})
    durations = sorted(sample["stop_to_all_exited_ns"] for sample in samples)
    result = {
        "schema": 1,
        "measured_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Wall time from starting the stop CLI until the run process exits after terminating all tracked Python process trees. No model calls. Includes stop-process launch, file propagation, taskkill/process-group termination and report output.",
        "machine": {"platform": platform.platform(), "machine": platform.machine(), "processor": platform.processor(), "logical_cpus": os.cpu_count()},
        "tasks_per_run": args.tasks,
        "samples": samples,
        "summary": {"median_ms": statistics.median(durations) / 1e6, "p95_ms": durations[max(0, (len(durations) * 95 + 99) // 100 - 1)] / 1e6, "max_ms": durations[-1] / 1e6},
    }
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(result["summary"], indent=2))


if __name__ == "__main__":
    main()
