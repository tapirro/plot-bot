#!/usr/bin/env python3
"""Work Queue — VERA-scored prioritization with feedback learning.

Collects actionable items from AWR sources, scores them by VERA formula,
applies feedback-based domain boosts, outputs ranked queue.

Sources:
  1. ./ask actions -j -q -s todo  → bet/maintenance actions
  2. work/intake/meetings/        → meeting action items (AI-assignable)
  3. ./ask @tensions -j -q        → orphan tensions → meta-tasks

Scoring: tension_severity × min(frequency,3) × recency/10 × meaning_coverage / cost × feedback_boost

Usage:
  python3 tools/scripts/work_queue.py --cli --top 8
  python3 tools/scripts/work_queue.py --json
  python3 tools/scripts/work_queue.py --apply '{"approved":[1,2],"reasoning":"focus hilart"}'
"""

from __future__ import annotations

import json
import math
import os
import re
import subprocess
import sys
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

# Paths
REPO_ROOT = Path(__file__).resolve().parent.parent.parent
ASK = str(REPO_ROOT / "ask")
MEETINGS_DIR = REPO_ROOT / "work" / "intake" / "meetings"
FEEDBACK_PATH = REPO_ROOT / "context" / "queue_feedback.jsonl"

# Non-AI owners (copied from serve.py — single source in serve.py)
NON_AI_OWNERS = {"катя", "люба", "дима", "санчусу", "саша_васильев",
                 "изабель", "дэниэл", "ольга", "kirill", "leonid"}

HUMAN_VERBS = ["inform ", "notify ", "discuss ", "talk ", "meet ",
               "call ", "написать ", "позвонить", "связаться",
               "поговорить", "обсудить", "найти общие чат",
               "завести", "организовать встреч", "спросить"]

# Vadim's identifiers — only include meetings where he participated
VADIM_IDS = {"vadim", "вадим", "vadim.mukhin", "vadim@umbrella-trade.com",
             "polansk", "vadim mukhin"}

AI_VERBS = [r"создать систем", r"разработать фреймворк", r"подготовить",
            r"собрать запис", r"проанализир", r"написать код", r"сгенерир",
            r"извлечь", r"структурир", r"систематизир", r"автоматизир",
            r"create", r"build", r"research", r"analyze", r"prepare",
            r"generate", r"extract", r"collect.*from", r"summarize",
            r"experiment with", r"implement"]


# ---------------------------------------------------------------------------
# Data
# ---------------------------------------------------------------------------

@dataclass
class QueueItem:
    """A single work item in the queue."""
    rank: int = 0
    source: str = ""        # "bet_action" | "intake" | "tension"
    source_id: str = ""     # e.g. "bet-hive-master#3" or "tension:bet-infra-mac-mini"
    title: str = ""
    domain: str = ""
    parent: str = ""        # parent bet/meeting name
    cost: str = "M"         # S/M/L/XL
    meanings: list[str] = field(default_factory=list)  # ["V","E","R","A"]
    severity: float = 5.0
    frequency: float = 1.0
    recency: float = 5.0    # 1-10, 10 = today
    score: float = 0.0
    score_breakdown: dict[str, float] = field(default_factory=dict)
    ai_doable: bool = True


# ---------------------------------------------------------------------------
# Collectors
# ---------------------------------------------------------------------------

def _run_ask(args: list[str]) -> Any:
    """Run ./ask with args, return parsed JSON or empty list."""
    try:
        r = subprocess.run(
            [ASK] + args + ["-j", "-q"],
            capture_output=True, text=True, timeout=10,
            cwd=str(REPO_ROOT),
        )
        return json.loads(r.stdout) if r.stdout.strip() else []
    except Exception:
        return []


def _is_ai_assignable(text: str, owner: str) -> bool:
    """Classify whether an action item can be handled by the AI assistant."""
    t = text.lower()
    if owner.lower() in NON_AI_OWNERS:
        return False
    for verb in HUMAN_VERBS:
        if verb in t:
            return False
    for verb in AI_VERBS:
        if re.search(verb, t):
            return True
    if owner.lower() in ("вадим", "vadim", "assistant", ""):
        ai_nouns = ["систем", "фреймворк", "сценари", "tool", "agent",
                    "bot", "script", "анализ", "report"]
        for noun in ai_nouns:
            if noun in t:
                return True
    return False


def _guess_cost(text: str) -> str:
    """Heuristic cost estimate from text length and keywords."""
    t = text.lower()
    if any(w in t for w in ["mvp", "полностью", "систем", "pipeline", "framework"]):
        return "L"
    if any(w in t for w in ["архитектур", "architecture", "design", "интеграц"]):
        return "L"
    if any(w in t for w in ["подготовить", "собрать", "create", "build", "implement"]):
        return "M"
    if any(w in t for w in ["проверить", "check", "review", "update", "fix"]):
        return "S"
    return "M"


def _guess_meanings(text: str, domain: str) -> list[str]:
    """Heuristic meaning tags from text content."""
    t = text.lower()
    meanings: list[str] = []
    # Value: delivery, product, user-facing
    if any(w in t for w in ["deliver", "product", "user", "клиент", "продукт", "доставк"]):
        meanings.append("V")
    # Elegance: tools, automation, scripts
    if any(w in t for w in ["tool", "script", "automat", "скрипт", "инструмент", "автоматиз"]):
        meanings.append("E")
    # Reliability: infra, deploy, monitoring
    if any(w in t for w in ["infra", "deploy", "monitor", "test", "тест", "деплой", "мониторинг"]):
        meanings.append("R")
    # Awareness: research, analysis, data
    if any(w in t for w in ["research", "analy", "data", "исследов", "анализ", "данн"]):
        meanings.append("A")
    if not meanings:
        meanings.append("E")  # default: most bot work is elegance
    return meanings


def _domain_from_parent(parent: str) -> str:
    """Extract domain hint from parent ID."""
    parts = parent.replace("bet-", "").replace("maintenance-", "").split("-")
    return parts[0] if parts else "unknown"


def collect_bet_actions() -> list[QueueItem]:
    """Collect open bet/maintenance actions assigned to AI."""
    raw = _run_ask(["actions", "-s", "todo"])
    actions = raw.get("actions", []) if isinstance(raw, dict) else raw if isinstance(raw, list) else []
    items: list[QueueItem] = []
    for a in actions:
        owner = a.get("owner", "")
        text = a.get("text", "")
        parent = a.get("parent", "")
        if not _is_ai_assignable(text, owner):
            continue
        domain = _domain_from_parent(parent)
        items.append(QueueItem(
            source="bet_action",
            source_id=f"{parent}#{a.get('index', 0)}",
            title=text,
            domain=domain,
            parent=parent,
            cost=_guess_cost(text),
            meanings=_guess_meanings(text, domain),
            severity=6.0,
            frequency=1.0,
            recency=5.0,
            ai_doable=True,
        ))
    return items


def _vadim_participated(content: str) -> bool:
    """Check if Vadim participated in a meeting (by participants field or body mentions)."""
    content_lower = content.lower()
    for vid in VADIM_IDS:
        if vid in content_lower:
            return True
    return False


def collect_intake_actions() -> list[QueueItem]:
    """Collect AI-assignable actions from recent meeting notes where Vadim participated."""
    if not MEETINGS_DIR.exists():
        return []
    items: list[QueueItem] = []
    # Only look at last 14 days of meetings
    cutoff = datetime.now().strftime("%Y-%m-")
    prev_month = (datetime.now().replace(day=1) - __import__("datetime").timedelta(days=1)).strftime("%Y-%m-")

    for f in sorted(MEETINGS_DIR.iterdir(), reverse=True):
        if not f.suffix == ".md":
            continue
        fname = f.name
        if not (fname.startswith(cutoff) or fname.startswith(prev_month)):
            continue
        try:
            content = f.read_text(encoding="utf-8")
        except Exception:
            continue
        # Only include meetings where Vadim participated
        if not _vadim_participated(content):
            continue
        # Extract action items (checkbox lines)
        in_actions = False
        for line in content.split("\n"):
            ll = line.lower()
            if "action" in ll and "#" in line:
                in_actions = True
                continue
            if line.startswith("#") and in_actions:
                in_actions = False
                continue
            if not in_actions:
                continue
            m = re.match(r"^- \[ \] (.+)", line)
            if not m:
                continue
            text = m.group(1).strip()
            # Check for owner tag
            owner_m = re.search(r"@(\w+)", text)
            owner = owner_m.group(1) if owner_m else ""
            if not _is_ai_assignable(text, owner):
                continue
            # Recency: days since meeting
            date_m = re.match(r"(\d{4}-\d{2}-\d{2})", fname)
            if date_m:
                days_ago = (datetime.now() - datetime.strptime(date_m.group(1), "%Y-%m-%d")).days
                recency = max(1.0, 10.0 - days_ago * 0.5)
            else:
                recency = 3.0

            items.append(QueueItem(
                source="intake",
                source_id=f"meeting:{fname}",
                title=text,
                domain="meeting",
                parent=fname.replace(".md", ""),
                cost=_guess_cost(text),
                meanings=_guess_meanings(text, ""),
                severity=4.0,
                frequency=1.0,
                recency=recency,
                ai_doable=True,
            ))
    return items


def collect_orphan_tensions() -> list[QueueItem]:
    """Collect tensions without active bets → meta-tasks to investigate."""
    raw = _run_ask(["@tensions"])
    tensions = raw if isinstance(raw, list) else []
    items: list[QueueItem] = []
    for t in tensions:
        bet_id = t.get("bet_id", "")
        pct = t.get("pct", 0)
        status = t.get("status", "")
        tension_text = t.get("tension", "")
        domain = t.get("domain", "unknown")
        # "Orphan" = proposed/stalled or 0% progress
        if status not in ("proposed",) and pct > 0:
            continue
        items.append(QueueItem(
            source="tension",
            source_id=f"tension:{bet_id}",
            title=f"Investigate: {tension_text[:120]}",
            domain=domain,
            parent=bet_id,
            cost="M",
            meanings=["A"],  # awareness: investigation
            severity=float(t.get("severity", 7)),
            frequency=1.0,
            recency=4.0,
            ai_doable=True,
        ))
    return items


# ---------------------------------------------------------------------------
# Scoring
# ---------------------------------------------------------------------------

COST_MAP = {"S": 1, "M": 2, "L": 4, "XL": 8}

MEANING_COVERAGE = {0: 0.5, 1: 1.0, 2: 1.5, 3: 2.5, 4: 4.0}


def score_item(item: QueueItem, feedback_boost: float = 1.0) -> float:
    """VERA score: severity × min(frequency,3) × recency/10 × meaning_coverage / cost × feedback_boost."""
    mc = MEANING_COVERAGE.get(len(item.meanings), 1.0)
    cost = COST_MAP.get(item.cost, 2)
    freq = min(item.frequency, 3.0)
    raw = (item.severity * freq * (item.recency / 10.0) * mc) / cost * feedback_boost
    item.score = round(raw, 2)
    item.score_breakdown = {
        "severity": item.severity,
        "frequency": freq,
        "recency": item.recency,
        "meaning_coverage": mc,
        "cost": cost,
        "feedback_boost": feedback_boost,
    }
    return item.score


# ---------------------------------------------------------------------------
# Feedback Store
# ---------------------------------------------------------------------------

class FeedbackStore:
    """Reads queue_feedback.jsonl, computes domain/type boosts."""

    def __init__(self, path: Path = FEEDBACK_PATH) -> None:
        self.path = path
        self.entries: list[dict] = []
        if path.exists():
            try:
                for line in path.read_text(encoding="utf-8").strip().split("\n"):
                    if line.strip():
                        self.entries.append(json.loads(line))
            except Exception:
                pass

    def domain_boost(self, domain: str) -> float:
        """Selection rate for domain over last 20 entries → [0.5, 1.5]."""
        recent = self.entries[-20:] if self.entries else []
        if not recent:
            return 1.0
        chosen_count = sum(1 for e in recent if domain in e.get("domains_chosen", []))
        rejected_count = sum(1 for e in recent if domain in e.get("domains_rejected", []))
        total = chosen_count + rejected_count
        if total == 0:
            return 1.0
        rate = chosen_count / total
        # Map [0, 1] → [0.5, 1.5]
        return 0.5 + rate

    def save_feedback(self, data: dict) -> None:
        """Append feedback entry to jsonl."""
        self.path.parent.mkdir(parents=True, exist_ok=True)
        entry = {
            "ts": datetime.now(timezone.utc).isoformat(),
            **data,
        }
        with open(self.path, "a", encoding="utf-8") as f:
            f.write(json.dumps(entry, ensure_ascii=False) + "\n")


# ---------------------------------------------------------------------------
# Dedup & Merge
# ---------------------------------------------------------------------------

def dedup_items(items: list[QueueItem]) -> list[QueueItem]:
    """Remove duplicates by source_id, keep highest-scored."""
    seen: dict[str, QueueItem] = {}
    for item in items:
        key = item.source_id
        if key not in seen or item.score > seen[key].score:
            seen[key] = item
    return list(seen.values())


# ---------------------------------------------------------------------------
# Main Pipeline
# ---------------------------------------------------------------------------

def generate_queue(top_n: int = 0) -> list[QueueItem]:
    """Full pipeline: collect → score → dedup → rank."""
    feedback = FeedbackStore()

    # Collect from all sources
    items: list[QueueItem] = []
    items.extend(collect_bet_actions())
    items.extend(collect_intake_actions())
    items.extend(collect_orphan_tensions())

    # Score
    for item in items:
        boost = feedback.domain_boost(item.domain)
        score_item(item, feedback_boost=boost)

    # Dedup and sort
    items = dedup_items(items)
    items.sort(key=lambda x: x.score, reverse=True)

    # Rank
    for i, item in enumerate(items, 1):
        item.rank = i

    # Top N
    if top_n > 0:
        items = items[:top_n]

    return items


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

MEANING_SYMBOLS = {"V": "V", "E": "E", "R": "R", "A": "A"}
COST_SYMBOLS = {"S": "S", "M": "M", "L": "L", "XL": "XL"}


def format_cli(items: list[QueueItem]) -> str:
    """Numbered table for terminal."""
    if not items:
        return "Queue is empty."

    lines = [
        f"{'#':>3}  {'Score':>6}  {'Cost':>4}  {'VERA':>4}  {'Domain':<20}  {'Title':<60}  Source",
        f"{'─'*3}  {'─'*6}  {'─'*4}  {'─'*4}  {'─'*20}  {'─'*60}  {'─'*20}",
    ]
    for item in items:
        meanings_str = "".join(item.meanings)
        title = item.title[:60]
        domain = item.domain[:20]
        source = item.source_id[:20]
        lines.append(
            f"{item.rank:>3}  {item.score:>6.1f}  {item.cost:>4}  {meanings_str:<4}  {domain:<20}  {title:<60}  {source}"
        )
    return "\n".join(lines)


def format_json(items: list[QueueItem]) -> str:
    """JSON output."""
    return json.dumps(
        {"queue": [asdict(it) for it in items], "generated": datetime.now(timezone.utc).isoformat()},
        ensure_ascii=False, indent=2,
    )


def apply_feedback(raw: str) -> None:
    """Parse and save feedback from --apply JSON string."""
    data = json.loads(raw)
    approved_ranks = data.get("approved", [])
    reasoning = data.get("reasoning", "")

    # Generate queue to map ranks to items
    items = generate_queue()
    approved_ids = []
    domains_chosen: set[str] = set()
    domains_rejected: set[str] = set()
    all_domains: set[str] = set()

    for item in items:
        all_domains.add(item.domain)
        if item.rank in approved_ranks:
            approved_ids.append(item.source_id)
            domains_chosen.add(item.domain)

    domains_rejected = all_domains - domains_chosen

    feedback = FeedbackStore()
    feedback.save_feedback({
        "items_offered": [it.source_id for it in items[:max(approved_ranks) if approved_ranks else 8]],
        "items_approved": approved_ids,
        "reasoning": reasoning,
        "domains_chosen": sorted(domains_chosen),
        "domains_rejected": sorted(domains_rejected),
    })
    print(f"Saved feedback: {len(approved_ids)} approved, reasoning: {reasoning}")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> None:
    import argparse
    parser = argparse.ArgumentParser(description="Work Queue — VERA-scored prioritization")
    parser.add_argument("--cli", action="store_true", help="Terminal table output")
    parser.add_argument("--json", action="store_true", dest="json_out", help="JSON output")
    parser.add_argument("--top", type=int, default=0, help="Limit to top N items")
    parser.add_argument("--apply", dest="apply_data", help="Apply feedback JSON string")
    args = parser.parse_args()

    if args.apply_data:
        apply_feedback(args.apply_data)
        return

    items = generate_queue(top_n=args.top)

    if args.json_out:
        print(format_json(items))
    else:
        # Default to CLI
        print(format_cli(items))


if __name__ == "__main__":
    main()
