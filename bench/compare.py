#!/usr/bin/env python3
"""Run local scheduler candidates and preserve every raw sample."""
import argparse
import json
import os
import platform
import statistics
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


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
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--skein-bench", required=True)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--tasks", type=int, nargs="+", default=[1000, 10000])
    parser.add_argument("--runs", type=int, default=9)
    parser.add_argument("--warmups", type=int, default=2)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    script = Path(__file__).with_name("python_candidate.py")
    report = {
        "schema": 1,
        "measured_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Independent no-op tasks; scheduler/control-plane only. No models, providers, network calls, or workload processes. Candidate process startup and LangGraph compilation are outside measured samples; Skein validates its in-memory manifest on every sample.",
        "warning": "Local observations, not universal rankings or agent-quality benchmarks. Python tracemalloc and Go allocation metrics are not directly comparable and must not be used as a cross-runtime memory ranking.",
        "machine": {
            "platform": platform.platform(),
            "machine": platform.machine(),
            "processor": platform.processor(),
            "logical_cpus": os.cpu_count(),
            "python": platform.python_version(),
        },
        "results": [],
    }
    for tasks in args.tasks:
        candidates = [
            ["skein", [args.skein_bench, "--tasks", str(tasks), "--runs", str(args.runs), "--warmups", str(args.warmups)]],
            ["asyncio", [args.python, str(script), "--candidate", "asyncio", "--tasks", str(tasks), "--runs", str(args.runs), "--warmups", str(args.warmups)]],
        ]
        try:
            importlib = __import__("importlib.util", fromlist=["find_spec"])
            if importlib.find_spec("langgraph") is not None:
                candidates.append(["langgraph", [args.python, str(script), "--candidate", "langgraph", "--tasks", str(tasks), "--runs", str(args.runs), "--warmups", str(args.warmups)]])
        except (ImportError, AttributeError):
            pass
        for name, command in candidates:
            completed = subprocess.run(command, check=True, capture_output=True, text=True, encoding="utf-8")
            raw = json.loads(completed.stdout)
            report["results"].append({"tasks": tasks, "command": command, "raw": raw, "summary": summarize(raw["samples"])})
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    for result in report["results"]:
        summary = result["summary"]
        print(f"{result['raw']['candidate']:10} {result['tasks']:7} tasks  median {summary['median_ms']:9.3f} ms  {summary['median_tasks_per_second']:12,.0f} tasks/s")


if __name__ == "__main__":
    main()
