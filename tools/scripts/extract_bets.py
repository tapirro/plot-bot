#!/usr/bin/env python3
"""extract_bets.py — Extract bet frontmatter from AWR markdown into JSON for tl sync.

Usage:
    python3 tools/scripts/extract_bets.py                    # scan work/bets/
    python3 tools/scripts/extract_bets.py --dir work/bets/   # explicit dir
    python3 tools/scripts/extract_bets.py --output /tmp/bets.json

Output format matches Telema2 POST /api/sync/push-bets schema:
    {"bets": [{"awr_id": "...", "domain": "...", "name": "...", ...}]}
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path


def parse_frontmatter(content: str) -> dict[str, str] | None:
    """Extract YAML frontmatter as a simple dict (no pyyaml dependency)."""
    if not content.startswith("---\n"):
        return None
    end = content.find("\n---\n", 4)
    if end == -1:
        end = content.find("\n---", 4)
        if end == -1:
            return None

    fm_text = content[4:end]
    result: dict[str, str] = {}
    for line in fm_text.split("\n"):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        match = re.match(r"^(\w[\w-]*)\s*:\s*(.+)$", line)
        if match:
            key = match.group(1)
            val = match.group(2).strip()
            # Strip quotes
            if (val.startswith('"') and val.endswith('"')) or (val.startswith("'") and val.endswith("'")):
                val = val[1:-1]
            # Parse list values like [tag1, tag2]
            if val.startswith("[") and val.endswith("]"):
                val = val  # keep as string, Telema2 handles it
            result[key] = val
    return result


def extract_title(content: str) -> str | None:
    """Extract first H1 heading from markdown content."""
    for line in content.split("\n"):
        if line.startswith("# "):
            return line[2:].strip()
    return None


def extract_block(content: str, header: str) -> str | None:
    """Extract text under a specific ## header."""
    pattern = rf"^## .*{re.escape(header)}.*$"
    lines = content.split("\n")
    capturing = False
    result: list[str] = []

    for line in lines:
        if re.match(pattern, line, re.IGNORECASE):
            capturing = True
            continue
        if capturing:
            if line.startswith("## "):
                break
            result.append(line)

    text = "\n".join(result).strip()
    return text if text else None


def scan_bets(bet_dir: Path) -> list[dict]:
    """Scan a directory for bet markdown files and extract frontmatter."""
    bets: list[dict] = []

    if not bet_dir.is_dir():
        return bets

    for md_file in sorted(bet_dir.rglob("*.md")):
        if md_file.name.startswith("INDEX") or md_file.name.startswith("_"):
            continue

        content = md_file.read_text(encoding="utf-8", errors="ignore")
        fm = parse_frontmatter(content)
        if fm is None:
            continue

        # Only process bet-type artifacts
        if fm.get("type") != "bet":
            continue

        awr_id = fm.get("id")
        if not awr_id:
            continue

        title = extract_title(content) or fm.get("name", awr_id)
        hypothesis = extract_block(content, "Hypothesis") or extract_block(content, "Гипотеза") or fm.get("hypothesis", "")
        tension = extract_block(content, "Tension") or extract_block(content, "Напряжение") or fm.get("tension")

        bet: dict = {
            "awr_id": awr_id,
            "domain": fm.get("domain", "unknown"),
            "name": title,
            "hypothesis": hypothesis,
        }

        if tension:
            bet["tension"] = tension
        if fm.get("horizon"):
            # Normalize partial dates: "2026-04" → "2026-04-01"
            h = fm["horizon"]
            if re.match(r"^\d{4}-\d{2}$", h):
                h = h + "-01"
            bet["horizon"] = h
        # Frontmatter uses "status" but API expects "bet_status"
        bet_status = fm.get("bet_status") or fm.get("status")
        if bet_status and bet_status in ("proposed", "experiment", "validated", "scaled"):
            bet["bet_status"] = bet_status

        # Parse meanings from tags or meanings field
        meanings = fm.get("meanings")
        if meanings and meanings.startswith("["):
            meanings_list = [m.strip() for m in meanings[1:-1].split(",") if m.strip()]
            if meanings_list:
                bet["meanings"] = meanings_list

        bets.append(bet)

    return bets


def main() -> None:
    """CLI entry point."""
    import argparse

    parser = argparse.ArgumentParser(description="Extract bet frontmatter for tl sync")
    parser.add_argument("--dir", "-d", default="work/bets", help="Directory to scan (default: work/bets)")
    parser.add_argument("--output", "-o", help="Write to file instead of stdout")
    args = parser.parse_args()

    bet_dir = Path(args.dir)
    bets = scan_bets(bet_dir)

    payload = {"bets": bets}

    if args.output:
        Path(args.output).parent.mkdir(parents=True, exist_ok=True)
        with open(args.output, "w") as f:
            json.dump(payload, f, indent=2, ensure_ascii=False)
        print(f"Extracted {len(bets)} bets → {args.output}", file=sys.stderr)
    else:
        print(json.dumps(payload, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
