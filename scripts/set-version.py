#!/usr/bin/env python3
"""Sync release version into build metadata and frontend."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def normalize(version: str) -> str:
    version = version.strip()
    if version.startswith("v") or version.startswith("V"):
        version = version[1:]
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?", version):
        raise SystemExit(f"invalid version: {version!r} (expected like 1.2.3)")
    return version


def replace_file(path: Path, pattern: str, repl: str, flags: int = 0) -> None:
    text = path.read_text(encoding="utf-8")
    new, n = re.subn(pattern, repl, text, count=1, flags=flags)
    if n != 1:
        raise SystemExit(f"failed to update {path}: pattern not found once")
    path.write_text(new, encoding="utf-8")


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit(f"usage: {sys.argv[0]} <version>")
    version = normalize(sys.argv[1])

    replace_file(
        ROOT / "build/config.yml",
        r'(?m)^(  version: ")[^"]*(" # The application version)$',
        rf"\g<1>{version}\2",
    )
    for plist in (
        ROOT / "build/darwin/Info.plist",
        ROOT / "build/darwin/Info.dev.plist",
    ):
        replace_file(
            plist,
            r"(<key>CFBundleVersion</key>\s*<string>)[^<]+(</string>)",
            rf"\g<1>{version}\2",
        )
        replace_file(
            plist,
            r"(<key>CFBundleShortVersionString</key>\s*<string>)[^<]+(</string>)",
            rf"\g<1>{version}\2",
        )

    info = ROOT / "build/windows/info.json"
    data = json.loads(info.read_text(encoding="utf-8"))
    data["fixed"]["file_version"] = version
    data["info"]["0000"]["ProductVersion"] = version
    info.write_text(json.dumps(data, indent="\t", ensure_ascii=False) + "\n", encoding="utf-8")

    replace_file(
        ROOT / "frontend/src/App.tsx",
        r"const APP_VERSION = '[^']*'",
        f"const APP_VERSION = '{version}'",
    )

    print(version)


if __name__ == "__main__":
    main()
