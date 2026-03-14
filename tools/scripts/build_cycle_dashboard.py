#!/usr/bin/env python3
"""Build cycle dashboard HTML from CYCLE_PROGRESS.md + state.json.

Generates a self-contained HTML file at devreports/cycle_dashboard.html
using the Mantissa Design System (base.css, theme.js, nav.js).

Usage:
    python3 tools/scripts/build_cycle_dashboard.py
"""

import json
import re
import subprocess
from datetime import datetime
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
PROGRESS_FILE = REPO / "work" / "CYCLE_PROGRESS.md"
STATE_FILE = REPO / "context" / "state.json"
HEARTBEAT_FILE = REPO / "context" / "heartbeat.json"
PAUSE_FILE = REPO / "context" / "agent_paused"
ROADMAP_FILE = REPO / "work" / "bets" / "plot_bot_roadmap.md"
OUTPUT_FILE = REPO / "devreports" / "cycle_dashboard.html"

# --- Data Parsing ---


def parse_progress() -> list[dict]:
    """Parse CYCLE_PROGRESS.md into list of cycle dicts."""
    if not PROGRESS_FILE.exists():
        return []

    cycles = []
    for line in PROGRESS_FILE.read_text().splitlines():
        line = line.strip()
        if not line.startswith("|") or line.startswith("| #") or line.startswith("|--"):
            continue
        parts = [p.strip() for p in line.split("|")[1:-1]]
        if len(parts) < 10:
            continue
        try:
            num = int(parts[0])
        except ValueError:
            continue

        impact_str = parts[5].strip()
        try:
            impact = int(impact_str)
        except ValueError:
            impact = None  # META cycles

        stars = [s.strip() for s in parts[6].split(",") if s.strip()]

        try:
            files = int(parts[7])
        except ValueError:
            files = 0

        try:
            escalations = int(parts[8])
        except ValueError:
            escalations = 0

        cycles.append({
            "num": num,
            "date": parts[1],
            "mode": parts[2],
            "type": parts[3],
            "title": parts[4],
            "impact": impact,
            "stars": stars,
            "files": files,
            "escalations": escalations,
            "commit": parts[9] if parts[9] != "—" else None,
        })

    return cycles


def parse_state() -> dict:
    """Read state.json."""
    if not STATE_FILE.exists():
        return {}
    return json.loads(STATE_FILE.read_text())


def parse_roadmap() -> dict:
    """Parse roadmap markdown for phase completion stats and individual tasks."""
    if not ROADMAP_FILE.exists():
        return {"phases": [], "total": 0, "done": 0, "tasks": []}

    text = ROADMAP_FILE.read_text()
    phases: list[dict] = []
    tasks: list[dict] = []
    current_phase = None
    current_section = None
    phase_done = 0
    phase_total = 0

    for line in text.splitlines():
        # Detect phase headers (## Phase N: ...)
        m = re.match(r"^## (Phase \d+[^#]*)", line)
        if m:
            if current_phase:
                phases.append({"name": current_phase, "done": phase_done, "total": phase_total})
            current_phase = m.group(1).strip()
            current_section = current_phase
            phase_done = 0
            phase_total = 0
            continue

        # Detect sub-section headers (### N.N ...)
        m = re.match(r"^### (\d+\.\d+ .*)", line)
        if m:
            if current_phase and phase_total > 0:
                phases.append({"name": current_phase, "done": phase_done, "total": phase_total})
                phase_done = 0
                phase_total = 0
            current_phase = m.group(1).strip()
            current_section = current_phase
            continue

        # Parse checkbox tasks
        m_done = re.match(r"^- \[x\] (.+)", line)
        m_todo = re.match(r"^- \[ \] (.+)", line)
        if m_done:
            phase_total += 1
            phase_done += 1
            tasks.append({
                "section": current_section or "?",
                "text": m_done.group(1).strip(),
                "done": True,
            })
        elif m_todo:
            phase_total += 1
            tasks.append({
                "section": current_section or "?",
                "text": m_todo.group(1).strip(),
                "done": False,
            })

    if current_phase and phase_total > 0:
        phases.append({"name": current_phase, "done": phase_done, "total": phase_total})

    total = sum(p["total"] for p in phases)
    done = sum(p["done"] for p in phases)

    return {"phases": phases, "total": total, "done": done, "tasks": tasks}


def git_log_recent(n: int = 10) -> list[dict]:
    """Get recent git commits."""
    try:
        result = subprocess.run(
            ["git", "log", f"--oneline", f"-{n}", "--format=%h|%s|%cr"],
            capture_output=True, text=True, timeout=5, cwd=str(REPO),
        )
        if result.returncode != 0:
            return []
        commits = []
        for line in result.stdout.strip().splitlines():
            parts = line.split("|", 2)
            if len(parts) == 3:
                commits.append({"hash": parts[0], "message": parts[1], "age": parts[2]})
        return commits
    except Exception:
        return []


# --- HTML Generation ---


def parse_heartbeat() -> dict:
    """Read heartbeat.json for live status."""
    if not HEARTBEAT_FILE.exists():
        return {"status": "unknown", "cycle": 0, "timestamp": ""}
    try:
        return json.loads(HEARTBEAT_FILE.read_text())
    except (json.JSONDecodeError, OSError):
        return {"status": "unknown", "cycle": 0, "timestamp": ""}


def _live_banner_html(hb: dict) -> str:
    """Generate live status banner HTML from heartbeat data."""
    status = hb.get("status", "unknown")
    cycle = hb.get("cycle", 0)
    ctype = hb.get("cycle_type", "?")
    ts = hb.get("timestamp", "")
    pid = hb.get("pid", 0)
    mode = hb.get("mode", "?")

    # Calculate elapsed time
    elapsed = ""
    if ts:
        try:
            start = datetime.fromisoformat(ts.replace("Z", "+00:00"))
            now = datetime.now(start.tzinfo) if start.tzinfo else datetime.utcnow()
            delta = now - start
            mins = int(delta.total_seconds() // 60)
            secs = int(delta.total_seconds() % 60)
            if mins > 0:
                elapsed = f"{mins}m {secs}s"
            else:
                elapsed = f"{secs}s"
        except (ValueError, TypeError):
            elapsed = ""

    if status == "running":
        return (
            f'<div class="live-banner live-running">'
            f'<span class="live-dot live-dot-running"></span>'
            f'<span class="live-label">Running cycle #{cycle}</span>'
            f'<span class="live-detail">{ctype} | {mode} mode | PID {pid}</span>'
            f'<span class="live-time">{elapsed}</span>'
            f'</div>'
        )
    elif status == "cooldown":
        cooldown_s = hb.get("cooldown_seconds", 300)
        return (
            f'<div class="live-banner live-cooldown">'
            f'<span class="live-dot live-dot-cooldown"></span>'
            f'<span class="live-label">Cooldown</span>'
            f'<span class="live-detail">After cycle #{cycle} | {cooldown_s}s pause</span>'
            f'<span class="live-time">{elapsed} ago</span>'
            f'</div>'
        )
    elif status == "completed":
        return (
            f'<div class="live-banner live-idle">'
            f'<span class="live-dot live-dot-idle"></span>'
            f'<span class="live-label">Completed cycle #{cycle}</span>'
            f'<span class="live-detail">{ctype} | awaiting next</span>'
            f'<span class="live-time">{elapsed} ago</span>'
            f'</div>'
        )
    elif status == "paused":
        return (
            f'<div class="live-banner live-paused">'
            f'<span class="live-dot live-dot-paused"></span>'
            f'<span class="live-label">Paused by operator</span>'
            f'<span class="live-detail">After cycle #{cycle} | waiting for resume</span>'
            f'<span class="live-time">{elapsed}</span>'
            f'</div>'
        )
    elif status in ("failed", "timeout"):
        exit_code = hb.get("exit_code", "?")
        return (
            f'<div class="live-banner live-failed">'
            f'<span class="live-dot live-dot-failed"></span>'
            f'<span class="live-label">{"Timed out" if status == "timeout" else "Failed"}: cycle #{cycle}</span>'
            f'<span class="live-detail">exit={exit_code}</span>'
            f'<span class="live-time">{elapsed} ago</span>'
            f'</div>'
        )
    else:
        return (
            f'<div class="live-banner live-idle">'
            f'<span class="live-dot live-dot-idle"></span>'
            f'<span class="live-label">Bot stopped</span>'
            f'<span class="live-detail">No heartbeat data</span>'
            f'<span class="live-time"></span>'
            f'</div>'
        )


def _agent_control_html(is_paused: bool) -> str:
    """Generate pause/start button HTML."""
    if is_paused:
        return (
            '<button class="agent-ctrl agent-ctrl-start" id="agent-ctrl" onclick="agentControl(\'resume\')">'
            '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>'
            'Start Agent'
            '</button>'
        )
    return (
        '<button class="agent-ctrl agent-ctrl-pause" id="agent-ctrl" onclick="agentControl(\'pause\')">'
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>'
        'Pause Agent'
        '</button>'
    )


def build_html(cycles: list[dict], state: dict, roadmap: dict, heartbeat: dict, is_paused: bool = False) -> str:
    """Generate the full dashboard HTML."""
    now = datetime.now().strftime("%Y-%m-%d %H:%M")

    # KPIs
    total_cycles = len(cycles)
    meta_cycles = sum(1 for c in cycles if c["type"] == "META" or c["type"] == "M")
    regular_cycles = total_cycles - meta_cycles
    impacts = [c["impact"] for c in cycles if c["impact"] is not None]
    avg_impact = sum(impacts) / len(impacts) if impacts else 0
    total_escalations = sum(c["escalations"] for c in cycles)
    total_files = sum(c["files"] for c in cycles)

    # North star distribution
    ns_counts = {"V": 0, "E": 0, "R": 0, "A": 0}
    for c in cycles:
        for s in c["stars"]:
            if s in ns_counts:
                ns_counts[s] += 1

    # Roadmap progress
    roadmap_pct = round(roadmap["done"] / roadmap["total"] * 100) if roadmap["total"] > 0 else 0

    # State
    cycle_pos = state.get("cycle_position", 0)
    next_type = "META" if cycle_pos == 0 else f"REGULAR (#{cycle_pos}/4)"

    # Impact distribution for sparkline
    impact_counts = {1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
    for i in impacts:
        if i in impact_counts:
            impact_counts[i] += 1

    # Cycle table rows
    table_rows = ""
    for c in reversed(cycles):
        impact_html = f'<span class="impact-{c["impact"] or 0}">{c["impact"] or "—"}</span>'
        stars_html = " ".join(f'<span class="m-badge m-badge-{"value" if s == "V" else "elegance" if s == "E" else "reliability" if s == "R" else "awareness"}">{s}</span>' for s in c["stars"])
        type_cls = {"META": "type-meta", "M": "type-meta", "RESEARCH": "type-research", "R": "type-research", "ANALYSIS": "type-analysis", "A": "type-analysis", "BUILD": "type-build", "B": "type-build", "ESCALATION": "type-escalation", "X": "type-escalation"}.get(c["type"], "")
        commit_html = f'<code>{c["commit"]}</code>' if c["commit"] else "—"
        table_rows += f"""<tr>
          <td class="mono">{c["num"]}</td>
          <td>{c["date"]}</td>
          <td>{c["mode"]}</td>
          <td><span class="type-pill {type_cls}">{c["type"]}</span></td>
          <td>{c["title"]}</td>
          <td class="center">{impact_html}</td>
          <td>{stars_html}</td>
          <td class="center">{c["files"]}</td>
          <td class="center">{c["escalations"]}</td>
          <td>{commit_html}</td>
        </tr>\n"""

    # Roadmap phase rows
    phase_rows = ""
    for p in roadmap["phases"]:
        if p["total"] == 0:
            continue
        pct = round(p["done"] / p["total"] * 100) if p["total"] > 0 else 0
        bar_color = "var(--m-value)" if pct == 100 else "var(--m-elegance)" if pct > 50 else "var(--m-awareness)"
        phase_rows += f"""<div class="phase-row">
          <div class="phase-name">{p["name"]}</div>
          <div class="phase-bar-track"><div class="phase-bar-fill" style="width:{pct}%;background:{bar_color}"></div></div>
          <div class="phase-count">{p["done"]}/{p["total"]}</div>
        </div>\n"""

    # Backlog: group tasks by section
    all_tasks = roadmap.get("tasks", [])
    todo_tasks = [t for t in all_tasks if not t["done"]]
    done_tasks = [t for t in all_tasks if t["done"]]
    sections_order: list[str] = []
    sections_map: dict[str, list[dict]] = {}
    for t in all_tasks:
        s = t["section"]
        if s not in sections_map:
            sections_order.append(s)
            sections_map[s] = []
        sections_map[s].append(t)

    backlog_html = ""
    for idx, sec in enumerate(sections_order):
        sec_tasks = sections_map[sec]
        sec_done = sum(1 for t in sec_tasks if t["done"])
        sec_total = len(sec_tasks)
        sec_pct = round(sec_done / sec_total * 100) if sec_total > 0 else 0
        all_done = sec_done == sec_total
        bar_color = "var(--m-value)" if all_done else "var(--m-elegance)" if sec_pct > 50 else "var(--m-awareness)"
        # Collapse done sections by default
        collapsed = "collapsed" if all_done else ""
        toggle_cls = "open" if not all_done else ""
        tasks_li = ""
        for t in sec_tasks:
            check_cls = "done" if t["done"] else ""
            check_icon = "&#10003;" if t["done"] else ""
            text_cls = "done" if t["done"] else ""
            # Escape HTML in task text
            safe_text = t["text"].replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
            tasks_li += f'<div class="backlog-task"><span class="backlog-check {check_cls}">{check_icon}</span><span class="backlog-text {text_cls}">{safe_text}</span></div>\n'

        backlog_html += f"""<div class="backlog-section" data-section="{idx}">
          <div class="backlog-header" onclick="toggleSection({idx})">
            <span class="backlog-toggle {toggle_cls}" id="toggle-{idx}">&#9654;</span>
            <span class="backlog-name">{sec}</span>
            <div class="backlog-bar"><div class="backlog-bar-fill" style="width:{sec_pct}%;background:{bar_color}"></div></div>
            <span class="backlog-count">{sec_done}/{sec_total}</span>
          </div>
          <div class="backlog-tasks {collapsed}" id="tasks-{idx}">{tasks_li}</div>
        </div>\n"""

    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Plot Bot — Cycle Dashboard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=Newsreader:opsz,wght@6..72,400;6..72,600&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
<style>
:root {{
  --c-page: #FCFCF4; --c-card: #F0EDE4; --c-ink: #191918; --c-muted: #7C7C87;
  --c-subtle: #E8E5DE; --c-highlight: #F5F2EA; --c-footer: #191918;
  --m-value: #C4A24D; --m-elegance: #3D9E8F; --m-reliability: #6B7B8D; --m-awareness: #8B7EC8;
  --m-value-light: #F5EDD4; --m-elegance-light: #D9F0EC; --m-reliability-light: #DDE3E8; --m-awareness-light: #E5E0F5;
  --m-value-text: #7A6420; --m-elegance-text: #14655A; --m-reliability-text: #3D4E5C; --m-awareness-text: #4E4188;
  --radius: 5px;
}}
@media (prefers-color-scheme: dark) {{
  :root {{
    --c-page: #141413; --c-card: #1E1E1D; --c-ink: #EDEDEC; --c-muted: #929292;
    --c-subtle: #2A2A29; --c-highlight: #222221; --c-footer: #0D0D0C;
    --m-value: #D4B86A; --m-elegance: #66B5A8; --m-reliability: #8FA0B0; --m-awareness: #A99ADB;
    --m-value-light: rgba(196,162,77,0.20); --m-elegance-light: rgba(61,158,143,0.20);
    --m-reliability-light: rgba(107,123,141,0.20); --m-awareness-light: rgba(139,126,200,0.20);
    --m-value-text: #F5EDD4; --m-elegance-text: #B8E0D9; --m-reliability-text: #DDE3E8; --m-awareness-text: #E5E0F5;
  }}
}}
* {{ margin:0; padding:0; box-sizing:border-box; }}
body {{ font-family:'Inter',system-ui,sans-serif; background:var(--c-page); color:var(--c-ink); padding:24px; max-width:1200px; margin:0 auto; }}
h1 {{ font-family:'Newsreader',Georgia,serif; font-size:28px; font-weight:600; letter-spacing:-0.01em; margin-bottom:4px; }}
.subtitle {{ color:var(--c-muted); font-size:13px; margin-bottom:24px; }}

/* KPIs */
.kpi-grid {{ display:grid; grid-template-columns:repeat(6, 1fr); gap:12px; margin-bottom:24px; }}
@media (max-width:800px) {{ .kpi-grid {{ grid-template-columns:repeat(3, 1fr); }} }}
.kpi {{ background:var(--c-card); border-radius:var(--radius); padding:14px 16px; }}
.kpi-label {{ font-size:10px; color:var(--c-muted); text-transform:uppercase; letter-spacing:0.08em; margin-bottom:6px; }}
.kpi-value {{ font-size:26px; font-weight:600; }}
.kpi-sub {{ font-size:11px; color:var(--c-muted); margin-top:2px; }}

/* Cards */
.grid-2 {{ display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-bottom:16px; }}
@media (max-width:800px) {{ .grid-2 {{ grid-template-columns:1fr; }} }}
.card {{ background:var(--c-card); border-radius:var(--radius); padding:16px 20px; }}
.card-title {{ font-family:'Newsreader',Georgia,serif; font-size:16px; font-weight:600; margin-bottom:12px; }}

/* VERA rings */
.vera-row {{ display:flex; gap:20px; justify-content:center; }}
.vera-item {{ text-align:center; }}
.vera-ring {{ width:64px; height:64px; border-radius:50%; display:flex; align-items:center; justify-content:center; font-family:'JetBrains Mono',monospace; font-size:18px; font-weight:600; margin-bottom:6px; }}
.vera-label {{ font-size:10px; color:var(--c-muted); text-transform:uppercase; letter-spacing:0.05em; }}

/* Roadmap phases */
.phase-row {{ display:flex; align-items:center; gap:10px; margin-bottom:8px; }}
.phase-name {{ font-size:11px; width:200px; flex-shrink:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }}
.phase-bar-track {{ flex:1; height:14px; background:var(--c-subtle); border-radius:3px; overflow:hidden; }}
.phase-bar-fill {{ height:100%; border-radius:3px; transition:width 300ms; }}
.phase-count {{ font-size:11px; font-family:'JetBrains Mono',monospace; color:var(--c-muted); width:40px; text-align:right; flex-shrink:0; }}

/* Cycle table */
.tbl {{ width:100%; border-collapse:collapse; font-size:12px; margin-top:8px; }}
.tbl th {{ text-align:left; color:var(--c-muted); font-weight:400; padding:8px 6px; border-bottom:1px solid var(--c-subtle); font-size:11px; text-transform:uppercase; letter-spacing:0.05em; }}
.tbl td {{ padding:6px 6px; border-bottom:1px solid var(--c-subtle); vertical-align:middle; }}
.tbl tr:hover {{ background:var(--c-highlight); }}
.tbl .center {{ text-align:center; }}
.tbl .mono {{ font-family:'JetBrains Mono',monospace; font-size:11px; }}
.tbl code {{ font-family:'JetBrains Mono',monospace; font-size:10px; background:var(--c-subtle); padding:1px 5px; border-radius:3px; }}

/* Type pills */
.type-pill {{ display:inline-block; font-size:10px; font-weight:600; padding:2px 8px; border-radius:10px; }}
.type-meta {{ background:var(--m-awareness-light); color:var(--m-awareness-text); }}
.type-research {{ background:var(--m-elegance-light); color:var(--m-elegance-text); }}
.type-analysis {{ background:var(--m-value-light); color:var(--m-value-text); }}
.type-build {{ background:var(--m-reliability-light); color:var(--m-reliability-text); }}
.type-escalation {{ background:rgba(199,93,74,0.15); color:#C75D4A; }}

/* Impact colors */
.impact-1,.impact-2 {{ color:var(--c-muted); }}
.impact-3 {{ color:var(--m-reliability); }}
.impact-4 {{ color:var(--m-elegance); }}
.impact-5 {{ color:var(--m-value); font-weight:600; }}

/* VERA badges */
.m-badge {{ display:inline-block; font-size:9px; font-weight:600; padding:1px 6px; border-radius:8px; margin-right:2px; }}
.m-badge-value {{ background:var(--m-value-light); color:var(--m-value-text); }}
.m-badge-elegance {{ background:var(--m-elegance-light); color:var(--m-elegance-text); }}
.m-badge-reliability {{ background:var(--m-reliability-light); color:var(--m-reliability-text); }}
.m-badge-awareness {{ background:var(--m-awareness-light); color:var(--m-awareness-text); }}

/* Status bar */
.status {{ display:flex; gap:16px; align-items:center; padding:10px 16px; background:var(--c-card); border-radius:var(--radius); margin-bottom:24px; font-size:12px; }}
.status-dot {{ width:8px; height:8px; border-radius:50%; }}
.status-green {{ background:#3D9E8F; }}
.status-yellow {{ background:#C4A24D; }}
.status-red {{ background:#C75D4A; }}

/* Live status banner */
.live-banner {{ display:flex; align-items:center; gap:12px; padding:12px 20px; border-radius:var(--radius); margin-bottom:16px; font-size:13px; }}
.live-running {{ background:rgba(61,158,143,0.12); border:1px solid rgba(61,158,143,0.25); }}
.live-cooldown {{ background:rgba(196,162,77,0.12); border:1px solid rgba(196,162,77,0.25); }}
.live-idle {{ background:var(--c-card); border:1px solid var(--c-subtle); }}
.live-failed {{ background:rgba(199,93,74,0.12); border:1px solid rgba(199,93,74,0.25); }}
.live-dot {{ width:10px; height:10px; border-radius:50%; flex-shrink:0; }}
.live-dot-running {{ background:#3D9E8F; animation:pulse 2s infinite; }}
.live-dot-cooldown {{ background:#C4A24D; }}
.live-dot-idle {{ background:var(--c-muted); }}
.live-dot-failed {{ background:#C75D4A; }}
.live-paused {{ background:rgba(196,162,77,0.12); border:1px solid rgba(196,162,77,0.25); }}
.live-dot-paused {{ background:#C4A24D; animation:pulse 3s infinite; }}
@keyframes pulse {{ 0%,100% {{ opacity:1; }} 50% {{ opacity:0.4; }} }}
.live-label {{ font-weight:500; }}
.live-detail {{ color:var(--c-muted); }}
.live-time {{ font-family:'JetBrains Mono',monospace; font-size:12px; color:var(--c-muted); margin-left:auto; }}

/* Agent control button */
.agent-ctrl {{ display:inline-flex; align-items:center; gap:6px; padding:6px 16px; border-radius:var(--radius); border:1px solid var(--c-subtle); background:var(--c-card); color:var(--c-ink); font-size:12px; font-weight:500; cursor:pointer; transition:all 150ms; margin-left:12px; }}
.agent-ctrl:hover {{ background:var(--c-highlight); border-color:var(--c-muted); }}
.agent-ctrl-pause {{ border-color:rgba(199,93,74,0.4); }}
.agent-ctrl-pause:hover {{ background:rgba(199,93,74,0.08); }}
.agent-ctrl-start {{ border-color:rgba(61,158,143,0.4); }}
.agent-ctrl-start:hover {{ background:rgba(61,158,143,0.08); }}
.agent-ctrl:disabled {{ opacity:0.5; cursor:not-allowed; }}
.agent-ctrl svg {{ width:14px; height:14px; }}

/* Backlog */
.backlog-section {{ margin-bottom:8px; }}
.backlog-header {{ display:flex; align-items:center; gap:8px; padding:8px 0 4px; cursor:pointer; user-select:none; }}
.backlog-header:hover {{ opacity:0.8; }}
.backlog-toggle {{ font-size:10px; color:var(--c-muted); transition:transform 150ms; }}
.backlog-toggle.open {{ transform:rotate(90deg); }}
.backlog-name {{ font-size:12px; font-weight:600; }}
.backlog-count {{ font-size:10px; color:var(--c-muted); font-family:'JetBrains Mono',monospace; }}
.backlog-bar {{ flex:1; height:6px; background:var(--c-subtle); border-radius:3px; max-width:120px; overflow:hidden; }}
.backlog-bar-fill {{ height:100%; border-radius:3px; }}
.backlog-tasks {{ padding-left:20px; }}
.backlog-tasks.collapsed {{ display:none; }}
.backlog-task {{ display:flex; align-items:flex-start; gap:6px; padding:3px 0; font-size:11px; line-height:1.4; }}
.backlog-check {{ flex-shrink:0; width:14px; height:14px; border:1.5px solid var(--c-subtle); border-radius:3px; margin-top:1px; display:flex; align-items:center; justify-content:center; }}
.backlog-check.done {{ background:var(--m-elegance); border-color:var(--m-elegance); color:#fff; font-size:9px; }}
.backlog-text {{ color:var(--c-ink); }}
.backlog-text.done {{ color:var(--c-muted); text-decoration:line-through; }}
.backlog-summary {{ display:flex; gap:16px; font-size:12px; color:var(--c-muted); margin-bottom:12px; padding-bottom:8px; border-bottom:1px solid var(--c-subtle); }}
.backlog-summary strong {{ color:var(--c-ink); }}
.backlog-filter {{ display:flex; gap:6px; margin-bottom:12px; }}
.backlog-filter-btn {{ font-size:10px; padding:3px 10px; border-radius:10px; border:1px solid var(--c-subtle); background:none; color:var(--c-muted); cursor:pointer; }}
.backlog-filter-btn.active {{ background:var(--c-ink); color:var(--c-page); border-color:var(--c-ink); }}
</style>
</head>
<body>

<div style="display:flex;align-items:baseline;gap:12px;">
<h1>Plot Bot</h1>
{_agent_control_html(is_paused)}
</div>
<p class="subtitle">Cycle Dashboard — generated {now}</p>

<!-- Live Status -->
{_live_banner_html(heartbeat)}

<!-- Status -->
<div class="status">
  <span class="status-dot {"status-green" if avg_impact >= 3 else "status-yellow" if avg_impact >= 2 else "status-red"}"></span>
  <span>Next: <strong>{next_type}</strong></span>
  <span>|</span>
  <span>Cycles: <strong>{total_cycles}</strong></span>
  <span>|</span>
  <span>Mega-cycles: <strong>{state.get("mega_cycle", 0)}</strong></span>
  <span>|</span>
  <span>Avg impact: <strong>{avg_impact:.1f}</strong></span>
  <span>|</span>
  <span>Roadmap: <strong>{roadmap_pct}%</strong> ({roadmap["done"]}/{roadmap["total"]})</span>
</div>

<!-- KPIs -->
<div class="kpi-grid">
  <div class="kpi">
    <div class="kpi-label">Cycles</div>
    <div class="kpi-value">{total_cycles}</div>
    <div class="kpi-sub">{meta_cycles} META + {regular_cycles} regular</div>
  </div>
  <div class="kpi">
    <div class="kpi-label">Avg Impact</div>
    <div class="kpi-value">{avg_impact:.1f}</div>
    <div class="kpi-sub">target &ge; 3.0</div>
  </div>
  <div class="kpi">
    <div class="kpi-label">Escalations</div>
    <div class="kpi-value">{total_escalations}</div>
    <div class="kpi-sub">to Vadim</div>
  </div>
  <div class="kpi">
    <div class="kpi-label">Files Changed</div>
    <div class="kpi-value">{total_files}</div>
    <div class="kpi-sub">across all cycles</div>
  </div>
  <div class="kpi">
    <div class="kpi-label">Roadmap</div>
    <div class="kpi-value">{roadmap_pct}%</div>
    <div class="kpi-sub">{roadmap["done"]}/{roadmap["total"]} tasks</div>
  </div>
  <div class="kpi">
    <div class="kpi-label">Position</div>
    <div class="kpi-value">{cycle_pos}/4</div>
    <div class="kpi-sub">next: {next_type.lower()}</div>
  </div>
</div>

<!-- VERA + Roadmap -->
<div class="grid-2">
  <div class="card">
    <div class="card-title">North Stars (VERA)</div>
    <div class="vera-row">
      <div class="vera-item">
        <div class="vera-ring" style="background:var(--m-value-light);color:var(--m-value-text);">{ns_counts["V"]}</div>
        <div class="vera-label">Value</div>
      </div>
      <div class="vera-item">
        <div class="vera-ring" style="background:var(--m-elegance-light);color:var(--m-elegance-text);">{ns_counts["E"]}</div>
        <div class="vera-label">Elegance</div>
      </div>
      <div class="vera-item">
        <div class="vera-ring" style="background:var(--m-reliability-light);color:var(--m-reliability-text);">{ns_counts["R"]}</div>
        <div class="vera-label">Reliability</div>
      </div>
      <div class="vera-item">
        <div class="vera-ring" style="background:var(--m-awareness-light);color:var(--m-awareness-text);">{ns_counts["A"]}</div>
        <div class="vera-label">Awareness</div>
      </div>
    </div>
  </div>
  <div class="card">
    <div class="card-title">Roadmap Progress</div>
    {phase_rows if phase_rows else '<p style="color:var(--c-muted);font-size:12px;">No roadmap data</p>'}
  </div>
</div>

<!-- Backlog -->
<div class="card" style="margin-bottom:16px;">
  <div class="card-title">Backlog</div>
  <div class="backlog-summary">
    <span><strong>{len(todo_tasks)}</strong> open</span>
    <span><strong>{len(done_tasks)}</strong> done</span>
    <span><strong>{len(all_tasks)}</strong> total</span>
    <span>{roadmap_pct}% complete</span>
  </div>
  <div class="backlog-filter">
    <button class="backlog-filter-btn active" onclick="filterBacklog('all')">All</button>
    <button class="backlog-filter-btn" onclick="filterBacklog('open')">Open</button>
    <button class="backlog-filter-btn" onclick="filterBacklog('done')">Done</button>
  </div>
  {backlog_html if backlog_html else '<p style="color:var(--c-muted);font-size:12px;">No backlog data</p>'}
</div>

<!-- Cycle Log -->
<div class="card">
  <div class="card-title">Cycle Log</div>
  <table class="tbl">
    <thead>
      <tr>
        <th>#</th><th>Date</th><th>Mode</th><th>Type</th><th>Title</th>
        <th class="center">Impact</th><th>Stars</th><th class="center">Files</th>
        <th class="center">Esc</th><th>Commit</th>
      </tr>
    </thead>
    <tbody>
      {table_rows if table_rows else '<tr><td colspan="10" style="color:var(--c-muted);text-align:center;">No cycles yet</td></tr>'}
    </tbody>
  </table>
</div>

<script>
// Auto-refresh: poll /api/status every 15s and update live banner
(function() {{
  const banner = document.querySelector('.live-banner');
  if (!banner) return;
  const startTime = banner.querySelector('.live-time');

  // Parse initial timestamp from data attribute
  const initialTs = '{heartbeat.get("timestamp", "")}';
  const initialStatus = '{heartbeat.get("status", "unknown")}';

  // Update elapsed time every second if running
  if (initialStatus === 'running' && initialTs) {{
    const start = new Date(initialTs);
    setInterval(() => {{
      const now = new Date();
      const delta = Math.floor((now - start) / 1000);
      const m = Math.floor(delta / 60);
      const s = delta % 60;
      if (startTime) startTime.textContent = m > 0 ? m + 'm ' + s + 's' : s + 's';
    }}, 1000);
  }}

  // Poll /api/status for live updates (only works when served via --serve)
  async function pollStatus() {{
    try {{
      const r = await fetch('/api/status');
      if (!r.ok) return;
      const d = await r.json();
      // Full page reload when status changes — simple and correct
      if (d.status !== initialStatus || d.cycle !== {heartbeat.get("cycle", 0)}) {{
        location.reload();
      }}
    }} catch(e) {{
      // Not served via --serve, polling disabled
    }}
  }}
  setInterval(pollStatus, 15000);
}})();

// Backlog: toggle sections
function toggleSection(idx) {{
  const tasks = document.getElementById('tasks-' + idx);
  const toggle = document.getElementById('toggle-' + idx);
  if (!tasks || !toggle) return;
  tasks.classList.toggle('collapsed');
  toggle.classList.toggle('open');
}}

// Backlog: filter
function filterBacklog(mode) {{
  document.querySelectorAll('.backlog-filter-btn').forEach(b => b.classList.remove('active'));
  event.target.classList.add('active');
  document.querySelectorAll('.backlog-task').forEach(t => {{
    const isDone = t.querySelector('.backlog-check.done') !== null;
    if (mode === 'all') t.style.display = '';
    else if (mode === 'open') t.style.display = isDone ? 'none' : '';
    else if (mode === 'done') t.style.display = isDone ? '' : 'none';
  }});
  // Hide empty sections
  document.querySelectorAll('.backlog-section').forEach(s => {{
    const visible = s.querySelectorAll('.backlog-task:not([style*="display: none"])');
    s.style.display = visible.length === 0 ? 'none' : '';
  }});
}}

// Agent control: pause / resume
async function agentControl(action) {{
  const btn = document.getElementById('agent-ctrl');
  if (btn) btn.disabled = true;
  try {{
    const r = await fetch('/api/agent/' + action, {{ method: 'POST' }});
    if (r.ok) {{
      location.reload();
    }} else {{
      const d = await r.json().catch(() => ({{}}));
      alert('Error: ' + (d.error || r.statusText));
      if (btn) btn.disabled = false;
    }}
  }} catch(e) {{
    alert('Dashboard not in serve mode — control unavailable');
    if (btn) btn.disabled = false;
  }}
}}
</script>

</body>
</html>"""


def build_once() -> None:
    """Build dashboard once."""
    OUTPUT_FILE.parent.mkdir(parents=True, exist_ok=True)

    cycles = parse_progress()
    state = parse_state()
    heartbeat = parse_heartbeat()
    roadmap = parse_roadmap()
    paused = PAUSE_FILE.exists()

    html = build_html(cycles, state, roadmap, heartbeat, is_paused=paused)
    OUTPUT_FILE.write_text(html)
    print(f"Dashboard built: {OUTPUT_FILE}")
    print(f"  Cycles: {len(cycles)}, Roadmap: {roadmap['done']}/{roadmap['total']}")


def serve(port: int = 3000) -> None:
    """Serve dashboard with auto-rebuild on each request.

    GET /             → rebuilds and serves cycle_dashboard.html
    GET /api/status   → returns heartbeat.json + state.json merged
    """
    import http.server

    class Handler(http.server.BaseHTTPRequestHandler):
        def _json_response(self, code: int, data: dict) -> None:
            body = json.dumps(data).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Access-Control-Allow-Origin", "*")
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self) -> None:
            path = self.path.split("?")[0].rstrip("/") or "/"

            if path == "/api/status":
                hb = parse_heartbeat()
                st = parse_state()
                data = {**hb, "state": st, "paused": PAUSE_FILE.exists()}
                self._json_response(200, data)
                return

            if path == "/" or path == "/dashboard":
                # Rebuild on every request for live data
                cycles = parse_progress()
                state = parse_state()
                heartbeat = parse_heartbeat()
                roadmap = parse_roadmap()
                paused = PAUSE_FILE.exists()
                html = build_html(cycles, state, roadmap, heartbeat, is_paused=paused)
                body = html.encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.end_headers()
                self.wfile.write(body)
                return

            self.send_response(404)
            self.end_headers()

        def do_POST(self) -> None:
            path = self.path.split("?")[0].rstrip("/")

            if path == "/api/agent/pause":
                PAUSE_FILE.write_text(
                    f'{{"paused_at": "{datetime.now().isoformat()}", "by": "dashboard"}}\n'
                )
                self._json_response(200, {"ok": True, "paused": True})
                return

            if path == "/api/agent/resume":
                if PAUSE_FILE.exists():
                    PAUSE_FILE.unlink()
                self._json_response(200, {"ok": True, "paused": False})
                return

            self._json_response(404, {"error": "not found"})

        def log_message(self, fmt: str, *args: object) -> None:
            pass  # Quiet

    print(f"Serving dashboard on http://localhost:{port}")
    print(f"  Dashboard: http://localhost:{port}/")
    print(f"  Status API: http://localhost:{port}/api/status")
    http.server.ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()


def main() -> None:
    import sys
    if "--serve" in sys.argv:
        port = 3000
        for i, a in enumerate(sys.argv):
            if a == "--port" and i + 1 < len(sys.argv):
                port = int(sys.argv[i + 1])
        serve(port)
    else:
        build_once()


if __name__ == "__main__":
    main()
