#!/usr/bin/env python3
"""Run rotated batches of equivalent no-op DAGs and preserve every sample."""
import argparse
import importlib.util
import json
import os
import platform
import statistics
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


def percentile_nearest_rank(sorted_values, percentile):
    return sorted_values[max(0, (len(sorted_values) * percentile + 99) // 100 - 1)]


def summarize(samples):
    durations = sorted(sample["duration_ns"] for sample in samples)
    rates = [sample["tasks_per_second"] for sample in samples]
    median = statistics.median(durations)
    return {
        "runs": len(samples),
        "min_ms": durations[0] / 1e6,
        "median_ms": median / 1e6,
        "median_absolute_deviation_ms": statistics.median(abs(value - median) for value in durations) / 1e6,
        "p95_ms": percentile_nearest_rank(durations, 95) / 1e6,
        "max_ms": durations[-1] / 1e6,
        "median_tasks_per_second": statistics.median(rates),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--skein-bench", required=True)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--shape", choices=["independent", "chain", "fanout"], default="independent")
    parser.add_argument("--tasks", type=int, nargs="+", default=[100, 1000])
    parser.add_argument("--runs", type=int, default=5, help="measured runs per candidate per batch")
    parser.add_argument("--batches", type=int, default=3)
    parser.add_argument("--warmups", type=int, default=2, help="warmups per candidate per batch")
    parser.add_argument("--langgraph-max", type=int, default=1000)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if args.runs < 1 or args.batches < 1 or args.warmups < 0:
        parser.error("runs and batches must be positive; warmups cannot be negative")
    script = Path(__file__).with_name("python_candidate.py")
    langgraph_available = importlib.util.find_spec("langgraph") is not None
    report = {
        "schema": 2,
        "measured_at": datetime.now(timezone.utc).isoformat(),
        "shape": args.shape,
        "scope": "Equivalent no-op DAGs; scheduler/control-plane only. No models, providers, network calls, or workload processes. Candidate process startup and LangGraph compilation are outside measured samples; Skein constructs its manifest once per batch and validates it on every sample.",
        "method": f"{args.batches} batches per candidate; candidate order rotates between batches. Each batch has {args.warmups} warmups followed by {args.runs} measured runs. Samples from batches are pooled. No tracemalloc or other memory profiler runs inside the timed region. The raw Go control averages 10 graph executions per sample to stay above the Windows timer floor; other candidates execute one graph per sample. p95 uses nearest rank.",
        "warning": "Local observations, not universal rankings or agent-quality benchmarks. Raw Go goroutines are an unstructured lower-bound control without graph validation, leases, reports, audit events, or stop fencing. LangGraph has a broader state/graph feature set. Different no-op DAG shapes must not be compared as if they were real model workloads.",
        "machine": {
            "platform": platform.platform(),
            "machine": platform.machine(),
            "processor": platform.processor(),
            "logical_cpus": os.cpu_count(),
            "python": platform.python_version(),
        },
        "results": [],
        "skipped": [],
    }
    grouped = {}
    for tasks in args.tasks:
        if tasks < 1 or (args.shape == "fanout" and tasks < 3):
            parser.error(f"invalid task count {tasks} for {args.shape}")
        candidates = [
            ("go", [args.skein_bench, "--candidate", "go", "--shape", args.shape, "--tasks", str(tasks)]),
            ("skein", [args.skein_bench, "--candidate", "skein", "--shape", args.shape, "--tasks", str(tasks)]),
            ("asyncio", [args.python, str(script), "--candidate", "asyncio", "--shape", args.shape, "--tasks", str(tasks)]),
        ]
        if langgraph_available and tasks <= args.langgraph_max:
            candidates.append(("langgraph", [args.python, str(script), "--candidate", "langgraph", "--shape", args.shape, "--tasks", str(tasks)]))
        elif langgraph_available:
            report["skipped"].append({"shape": args.shape, "tasks": tasks, "candidate": "langgraph", "reason": f"above configured graph-size limit {args.langgraph_max}; no extrapolation"})

        for batch in range(args.batches):
            rotated = candidates[batch % len(candidates):] + candidates[:batch % len(candidates)]
            for name, base_command in rotated:
                command = base_command + ["--runs", str(args.runs), "--warmups", str(args.warmups)]
                completed = subprocess.run(command, check=True, capture_output=True, text=True, encoding="utf-8")
                raw = json.loads(completed.stdout)
                key = (tasks, name)
                if key not in grouped:
                    grouped[key] = {"command": command, "raw": raw, "samples": []}
                for sample in raw["samples"]:
                    sample["batch"] = batch + 1
                    sample["batch_run"] = sample["run"]
                    grouped[key]["samples"].append(sample)
        for tasks_key, name in [(tasks, candidate[0]) for candidate in candidates]:
            entry = grouped[(tasks_key, name)]
            for index, sample in enumerate(entry["samples"], 1):
                sample["run"] = index
            entry["raw"]["samples"] = entry["samples"]
            summary = summarize(entry["samples"])
            result = {"shape": args.shape, "tasks": tasks_key, "command": entry["command"], "raw": entry["raw"], "summary": summary}
            report["results"].append(result)
            print(f"{args.shape:11} {name:14} {tasks_key:7} tasks  n={summary['runs']:2}  median {summary['median_ms']:9.3f} ms  p95 {summary['p95_ms']:9.3f} ms")
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
