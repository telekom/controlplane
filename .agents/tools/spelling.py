#!/usr/bin/env python3
# Copyright 2026 Deutsche Telekom AG
#
# SPDX-License-Identifier: Apache-2.0

"""Run codespell via stdin and emit filename-safe JSONL findings."""

import argparse
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ignore-words", required=True)
    parser.add_argument("files", nargs="*")
    args = parser.parse_args()
    if shutil.which("codespell") is None:
        print("codespell is not installed; install it and add it to PATH", file=sys.stderr)
        return 2
    for name in args.files:
        result = subprocess.run(
            ["codespell", "--quiet-level=2", "--stdin-single-line",
             f"--ignore-words={args.ignore_words}", "-"],
            input=Path(name).read_bytes(), capture_output=True,
        )
        if result.returncode not in (0, 65) or result.stderr:
            print(
                f"codespell failed for {name!r}: "
                f"{result.stderr.decode(errors='replace').strip()} "
                f"(exit {result.returncode})",
                file=sys.stderr,
            )
            return 2
        output = result.stdout.decode("utf-8")
        for line in output.splitlines():
            match = re.fullmatch(r"(\d+): (.+)", line)
            if not match:
                print(f"malformed codespell output: {line!r}", file=sys.stderr)
                return 2
            print(json.dumps({"file": name, "line": int(match[1]), "detail": match[2]}))
        if result.returncode == 65 and not output:
            print(f"codespell reported findings without output for {name!r}", file=sys.stderr)
            return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
