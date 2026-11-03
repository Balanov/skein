#!/usr/bin/env python3
"""Exercise real CLI data flow, dependency isolation and validation."""
import argparse
import json
import platform
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path


APPEND_CODE = "import os,sys; data=sys.stdin.read(); print(f\"{data}:{os.environ['SKEIN_TASK_ID']}/{os.environ['SKEIN_PASS']}\",end='')"
FAIL_CODE = "import os,sys; task=os.environ['SKEIN_TASK_ID']; print(task,end=''); sys.exit(7 if task=='bad' else 0)"


def invoke(skein, root, manifest, include_tasks=True):
    manifest_path = root / "manifest.json"
    manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
    command = [skein, "run", "--state", str(root / "state")]
    if include_tasks:
        command.append("--include-tasks")
    command.append(str(manifest_path))
    completed = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", timeout=30)
    report = json.loads(completed.stdout) if completed.stdout.strip().startswith("{") else None
    return completed, report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--skein", required=True)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    scenarios = []

    with tempfile.TemporaryDirectory(prefix="skein-functional-") as directory:
        root = Path(directory)
        manifest = {
            "version": 1,
            "workers": 1,
            "blocks": {"append": {"argv": [args.python, "-c", APPEND_CODE], "timeout_ms": 5000}},
            "tasks": [
                {"id": "a", "block": "append", "input": "start", "passes": 2},
                {"id": "b", "block": "append", "needs": ["a"], "stdin_from": "a"},
            ],
        }
        completed, report = invoke(args.skein, root, manifest)
        output = Path(report["tasks"]["b"]["output_path"]).read_text(encoding="utf-8")
        events_path = Path(report["tasks"]["b"]["output_path"]).parents[1] / "events.jsonl"
        events = [json.loads(line) for line in events_path.read_text(encoding="utf-8").splitlines()]
        passed = completed.returncode == 0 and report["succeeded"] == 2 and output == "start:a/1:a/2:b/1"
        if not passed:
            raise RuntimeError(f"multi-pass proof failed: {completed.stdout}\n{completed.stderr}")
        scenarios.append({"name": "multi_pass_data_flow", "passed": passed, "expected_output": "start:a/1:a/2:b/1", "observed_output": output, "report": report, "events": events})

    with tempfile.TemporaryDirectory(prefix="skein-functional-") as directory:
        root = Path(directory)
        manifest = {
            "version": 1,
            "workers": 2,
            "blocks": {"work": {"argv": [args.python, "-c", FAIL_CODE], "timeout_ms": 5000}},
            "tasks": [
                {"id": "bad", "block": "work"},
                {"id": "child", "block": "work", "needs": ["bad"], "stdin_from": "bad"},
                {"id": "independent", "block": "work"},
            ],
        }
        completed, report = invoke(args.skein, root, manifest)
        statuses = {name: task["status"] for name, task in report["tasks"].items()}
        passed = completed.returncode == 3 and statuses == {"bad": "failed", "child": "skipped", "independent": "succeeded"}
        if not passed:
            raise RuntimeError(f"dependency-isolation proof failed: {completed.stdout}\n{completed.stderr}")
        scenarios.append({"name": "dependency_failure_isolation", "passed": passed, "expected_exit_code": 3, "observed_exit_code": completed.returncode, "statuses": statuses, "report": report})

    with tempfile.TemporaryDirectory(prefix="skein-functional-") as directory:
        root = Path(directory)
        manifest = {
            "version": 1,
            "blocks": {"noop": {"argv": [args.python, "-c", "pass"]}},
            "tasks": [{"id": "a", "block": "noop", "needs": ["b"]}, {"id": "b", "block": "noop", "needs": ["a"]}],
        }
        completed, report = invoke(args.skein, root, manifest, include_tasks=False)
        passed = completed.returncode != 0 and "cycle" in completed.stderr.lower() and report is None
        if not passed:
            raise RuntimeError(f"cycle-validation proof failed: {completed.stdout}\n{completed.stderr}")
        scenarios.append({"name": "cycle_rejected_before_execution", "passed": passed, "observed_exit_code": completed.returncode, "diagnostic": completed.stderr.strip()})

    result = {
        "schema": 1,
        "measured_at": datetime.now(timezone.utc).isoformat(),
        "scope": "Real Skein CLI and real Python workload processes. These scenarios prove behavior, not speed.",
        "machine": {"platform": platform.platform(), "python": platform.python_version()},
        "summary": {"scenarios": len(scenarios), "passed": sum(1 for scenario in scenarios if scenario["passed"])},
        "scenarios": scenarios,
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(json.dumps(result["summary"], indent=2))


if __name__ == "__main__":
    main()
