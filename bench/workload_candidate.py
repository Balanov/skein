#!/usr/bin/env python3
"""Cold-start candidate for real subprocess work; the parent measures wall time."""
import argparse
import asyncio
import importlib.metadata
import json
import platform
import sys


HASH_CODE = "import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest(),end='')"


async def main_async(args, payload, expected):
    semaphore = asyncio.Semaphore(args.workers)

    async def work(_state=None):
        async with semaphore:
            process = await asyncio.create_subprocess_exec(
                sys.executable, "-c", HASH_CODE,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            stdout, stderr = await process.communicate(payload)
            if process.returncode != 0 or stdout.decode("ascii") != expected:
                raise RuntimeError(f"workload verification failed: code={process.returncode}, stderr={stderr.decode(errors='replace')}")

    if args.candidate == "asyncio":
        await asyncio.gather(*(work() for _ in range(args.tasks)))
        return

    from langgraph.graph import END, StateGraph, START
    from typing_extensions import TypedDict

    class State(TypedDict, total=False):
        value: int

    graph = StateGraph(State)
    for index in range(args.tasks):
        name = f"task_{index:06d}"
        graph.add_node(name, work)
        graph.add_edge(START, name)
        graph.add_edge(name, END)
    compiled = graph.compile()
    await compiled.ainvoke({"value": 0}, config={"max_concurrency": args.workers})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate", choices=["asyncio", "langgraph"], required=True)
    parser.add_argument("--tasks", type=int, required=True)
    parser.add_argument("--workers", type=int, required=True)
    parser.add_argument("--payload", required=True)
    parser.add_argument("--expected", required=True)
    args = parser.parse_args()
    with open(args.payload, "rb") as payload_file:
        payload = payload_file.read()
    asyncio.run(main_async(args, payload, args.expected))
    version = importlib.metadata.version("langgraph") if args.candidate == "langgraph" else platform.python_version()
    print(json.dumps({"candidate": args.candidate, "candidate_version": version, "tasks": args.tasks, "verified": args.tasks}))


if __name__ == "__main__":
    main()
