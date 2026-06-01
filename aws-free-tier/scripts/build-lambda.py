#!/usr/bin/env python3
"""Build a Go Lambda custom-runtime bootstrap in a cross-platform way."""

from __future__ import annotations

import argparse
import os
import stat
import subprocess
from pathlib import Path


def run(command: list[str], cwd: Path, env: dict[str, str]) -> None:
    subprocess.run(command, cwd=cwd, env=env, check=True)


def main() -> None:
    parser = argparse.ArgumentParser(description="Build a Go Lambda bootstrap.")
    parser.add_argument("--source-dir", required=True, help="Go module directory to build.")
    parser.add_argument("--output-dir", required=True, help="Directory where bootstrap is written.")
    args = parser.parse_args()

    source_dir = Path(args.source_dir).resolve()
    output_dir = Path(args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    build_root = output_dir.parent
    env = os.environ.copy()
    env.setdefault("GOCACHE", str(build_root / ".gocache"))
    env.setdefault("GOMODCACHE", str(build_root / ".gomodcache"))
    env["GOOS"] = "linux"
    env["GOARCH"] = "arm64"
    env["CGO_ENABLED"] = "0"

    Path(env["GOCACHE"]).mkdir(parents=True, exist_ok=True)
    Path(env["GOMODCACHE"]).mkdir(parents=True, exist_ok=True)

    bootstrap = output_dir / "bootstrap"

    run(["go", "mod", "download"], cwd=source_dir, env=env)
    run(
        [
            "go",
            "build",
            "-tags",
            "lambda.norpc",
            "-trimpath",
            "-ldflags=-s -w",
            "-o",
            str(bootstrap),
            ".",
        ],
        cwd=source_dir,
        env=env,
    )

    try:
        bootstrap.chmod(bootstrap.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    except OSError:
        pass


if __name__ == "__main__":
    main()
