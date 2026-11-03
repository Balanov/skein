#!/usr/bin/env python3
"""Compare cold, verified runs that each dispatch real Python hash processes."""
import argparse
import hashlib
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


HASH_CODE = "import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest(),end='')"


def summarize(samples):
    durations = sorted(sample["duration_ns"] for sample in samples)
    rates = [sample["tasks_per_second"] for sample in samples]
    return {
        "runs": len(samples),
        "min_ms": durations[0] / 1e6,
        "median_ms": statistics.median(durations) / 1e6,
        "p95_ms": durations[max(0, (len(durations) * 95 + 99) // 100 - 1)] / 1e6,
        "max_ms": durations[-1] / 1e6,
        "median_tasks_per_second": statistics.median(rates),
        "verified_tasks": sum(sample["verified"] for sample in samples),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--skein", required=True)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--tasks", type=int, default=24)
    parser.add_argument("--workers", type=int, default=8)
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--batches", type=int, default=3)
    parser.add_argument("--warmups", type=int, default=1)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if min(args.tasks, args.workers, args.runs, args.batches) < 1 or args.warmups < 0:
        parser.error("invalid benchmark arguments")
    script = Path(__file__).with_name("workload_candidate.py")
    payload = bytes(range(256)) * 64
    expected = hashlib.sha256(payload).hexdigest()
    grouped = {name: [] for name in ("skein", "asyncio", "langgraph")}

    def invoke(candidate, batch, run, warmup):
        with tempfile.TemporaryDirectory(prefix="skein-workload-") as directory:
            root = Path(directory)
            payload_path = root / "payload.bin"
            payload_path.write_bytes(payload)
            if candidate == "skein":
                manifest = {
                    "version": 1,
                    "workers": args.workers,
                    "blocks": {"sha256": {"argv": [args.python, "-c", HASH_CODE], "timeout_ms": 30000}},
                    "tasks": [{"id": f"hash-{index:03d}", "block": "sha256", "input_file": str(payload_path)} for index in range(args.tasks)],
                }
                manifest_path = root / "manifest.json"
                manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
                command = [args.skein, "run", "--state", str(root / "state"), "--no-events", "--include-tasks", str(manifest_path)]
            else:
                command = [args.python, str(script), "--candidate", candidate, "--tasks", str(args.tasks), "--workers", str(args.workers), "--payload", str(payload_path), "--expected", expected]
            started = time.perf_counter_ns()
            completed = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", timeout=120)
            duration = time.perf_counter_ns() - started
            if completed.returncode != 0:
                raise RuntimeError(f"{candidate} failed: {completed.stdout}\n{completed.stderr}")
            result = json.loads(completed.stdout)
            if candidate == "skein":
                if result["succeeded"] != args.tasks:
                    raise RuntimeError(f"unexpected Skein report: {result}")
                for task in result["tasks"].values():
                    if Path(task["output_path"]).read_text(encoding="ascii") != expected:
                        raise RuntimeError(f"digest mismatch in {task['id']}")
                verified = args.tasks
            else:
                verified = result["verified"]
            if verified != args.tasks:
                raise RuntimeError(f"not all tasks verified for {candidate}: {verified}")
            if not warmup:
                grouped[candidate].append({"batch": batch, "run": run, "duration_ns": duration, "tasks_per_second": args.tasks / (duration / 1e9), "verified": verified})

    candidates = ["skein", "asyncio", "langgraph"]
    for batch in range(1, args.batches + 1):
        rotated = candidates[(batch - 1) % len(candidates):] + candidates[:(batch - 1) % len(candidates)]
        for candidate in rotated:
            for warmup in range(args.warmups):
                invoke(candidate, batch, 0, True)
        for run in range(1, args.runs + 1):
            rotated = candidates[(batch + run - 1) % len(candidates):] + candidates[:(batch + run - 1) % len(candidates)]
            for candidate in rotated:
                invoke(candidate, batch, run, False)

    results = []
    for candidate in candidates:
        samples = grouped[candidate]
        summary = summarize(samples)
        results.append({"candidate": candidate, "tasks": args.tasks, "workers": args.workers, "samples": samples, "summary": summary})
        print(f"{candidate:10} n={summary['runs']:2} median {summary['median_ms']:9.3f} ms p95 {summary['p95_ms']:9.3f} ms verified={summary['verified_tasks']}")
    report = {
        "schema": 1,
        "measured_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Cold end-to-end wall time for independent real subprocess tasks. Every child reads a 16 KiB payload from stdin, computes SHA-256, and returns it; every result is verified. All candidate process startup, imports, Skein manifest validation, LangGraph graph compilation, workload-process startup, orchestration and result verification inside the candidate are included. Parent-side verification is outside the timed interval.",
        "method": f"{args.batches} batches; {args.warmups} warmup per candidate per batch; {args.runs} measured fresh processes per candidate per batch. Candidate order rotates between runs. Each run dispatches {args.tasks} children with a concurrency limit of {args.workers}. p95 uses nearest rank.",
        "warning": "This is a small local process-dispatch workload, not a model-quality or provider-latency benchmark. Cold-start results and warm scheduler results answer different questions and must not be mixed.",
        "machine": {"platform": platform.platform(), "machine": platform.machine(), "processor": platform.processor(), "logical_cpus": os.cpu_count(), "python": platform.python_version()},
        "tasks_per_run": args.tasks,
        "workers": args.workers,
        "payload_bytes": len(payload),
        "expected_sha256": expected,
        "results": results,
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
