#!/usr/bin/env python3
"""Build cycle dashboard HTML from CYCLE_PROGRESS.md + state.json.

Generates a self-contained HTML file at devreports/cycle_dashboard.html
using the Mantissa Design System (base.css, theme.js, nav.js).

Usage:
    python3 tools/scripts/build_cycle_dashboard.py
"""

import glob
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
CONFIG_FILE = REPO / "context" / "agent_config.json"
ROADMAP_FILE = REPO / "work" / "bets" / "plot_bot_roadmap.md"
OUTPUT_FILE = REPO / "devreports" / "cycle_dashboard.html"
AGENT_PLIST_LABEL = "com.mantissa.plot-bot"
AGENT_PLIST_PATH = Path.home() / "Library" / "LaunchAgents" / f"{AGENT_PLIST_LABEL}.plist"
SESSIONS_DIR = Path.home() / ".claude" / "projects" / "-Users-polansk-Developer-mantissa-code-plot-bot"
CYCLE_REPORTS_DIR = REPO / "work" / "cycle_reports"
FEEDBACK_DIR = REPO / "work" / "feedback"
ESCALATIONS_DIR = REPO / "out"

# --- Data Parsing ---


def parse_agent_config() -> dict:
    """Read agent_config.json."""
    if not CONFIG_FILE.exists():
        return {}
    try:
        return json.loads(CONFIG_FILE.read_text())
    except (json.JSONDecodeError, OSError):
        return {}


def parse_rate_control() -> dict:
    """Get rate control data from claude_limits.mjs."""
    try:
        r = subprocess.run(
            ["node", str(Path.home() / ".claude" / "rate-control" / "claude_limits.mjs"), "--json"],
            capture_output=True, text=True, timeout=10,
            env={**__import__("os").environ, "PATH": "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"},
        )
        if r.returncode == 0 and r.stdout.strip():
            d = json.loads(r.stdout)
            w = d.get("windows", {}).get("7d", {})
            return {
                "pct": w.get("pct", 0),
                "spent": w.get("spent", 0),
                "limit": w.get("limit", 0),
                "days_left": w.get("days_left", 0),
            }
    except Exception:
        pass
    return {"pct": 0, "spent": 0, "limit": 0, "days_left": 0}


def parse_session_tokens() -> dict:
    """Sum token usage from all plot-bot JSONL sessions."""
    total_input = 0
    total_output = 0
    total_cache_read = 0
    session_count = 0
    current_input = 0
    current_output = 0

    jsonl_files = sorted(glob.glob(str(SESSIONS_DIR / "*.jsonl")))
    for fpath in jsonl_files:
        session_in = 0
        session_out = 0
        try:
            with open(fpath) as f:
                for line in f:
                    try:
                        d = json.loads(line)
                        u = d.get("message", {}).get("usage", {})
                        if u:
                            session_in += u.get("input_tokens", 0)
                            session_out += u.get("output_tokens", 0)
                            total_cache_read += u.get("cache_read_input_tokens", 0)
                    except (json.JSONDecodeError, KeyError):
                        pass
        except OSError:
            continue
        if session_in > 0 or session_out > 0:
            session_count += 1
            total_input += session_in
            total_output += session_out
            current_input = session_in
            current_output = session_out

    return {
        "total_input": total_input,
        "total_output": total_output,
        "total_cache_read": total_cache_read,
        "total": total_input + total_output,
        "sessions": session_count,
        "current_input": current_input,
        "current_output": current_output,
        "current_total": current_input + current_output,
    }


def parse_cycle_reports() -> list[dict]:
    """Parse all cycle reports from work/cycle_reports/."""
    reports: list[dict] = []
    if not CYCLE_REPORTS_DIR.exists():
        return reports
    for f in sorted(CYCLE_REPORTS_DIR.glob("CYCLE_*.md")):
        text = f.read_text()
        meta: dict = {}
        body_sections: dict[str, str] = {}
        # Parse YAML frontmatter
        if text.startswith("---"):
            parts = text.split("---", 2)
            if len(parts) >= 3:
                for line in parts[1].strip().splitlines():
                    if ":" in line:
                        k, v = line.split(":", 1)
                        meta[k.strip()] = v.strip().strip('"').strip("'")
                text = parts[2]
        # Parse markdown sections
        current_section = None
        current_lines: list[str] = []
        for line in text.splitlines():
            if line.startswith("## "):
                if current_section:
                    body_sections[current_section] = "\n".join(current_lines).strip()
                current_section = line[3:].strip()
                current_lines = []
            elif current_section:
                current_lines.append(line)
        if current_section:
            body_sections[current_section] = "\n".join(current_lines).strip()

        # Extract escalations as list
        escalations: list[str] = []
        esc_text = body_sections.get("Escalations", "")
        for eline in esc_text.splitlines():
            eline = eline.strip()
            if eline.startswith("- "):
                escalations.append(eline[2:].strip())

        reports.append({
            "file": f.name,
            "meta": meta,
            "sections": body_sections,
            "escalations": escalations,
            "cycle": int(meta.get("cycle", 0)),
            "title": meta.get("title", f.stem),
            "impact": meta.get("impact", "—"),
            "type": meta.get("cycle_type", "?"),
            "north_stars": [s.strip() for s in meta.get("north_stars", "").strip("[]").split(",") if s.strip()],
        })
    return reports


def parse_feedback() -> list[dict]:
    """Parse feedback files from work/feedback/."""
    items: list[dict] = []
    if not FEEDBACK_DIR.exists():
        return items
    for f in sorted(FEEDBACK_DIR.glob("FEEDBACK_*.md")):
        text = f.read_text()
        meta: dict = {}
        if text.startswith("---"):
            parts = text.split("---", 2)
            if len(parts) >= 3:
                for line in parts[1].strip().splitlines():
                    if ":" in line:
                        k, v = line.split(":", 1)
                        meta[k.strip()] = v.strip().strip('"').strip("'")
        items.append({"file": f.name, "meta": meta})
    return items


def parse_escalation_docs() -> list[dict]:
    """Parse escalation documents from out/escalation_*.md."""
    docs: list[dict] = []
    if not ESCALATIONS_DIR.exists():
        return docs
    for f in sorted(ESCALATIONS_DIR.glob("escalation_*.md")):
        text = f.read_text()
        meta: dict = {}
        body = text
        if text.startswith("---"):
            parts = text.split("---", 2)
            if len(parts) >= 3:
                for line in parts[1].strip().splitlines():
                    if ":" in line:
                        k, v = line.split(":", 1)
                        meta[k.strip()] = v.strip().strip('"').strip("'")
                body = parts[2].strip()
        # Parse sections
        sections: dict[str, str] = {}
        current_sec = None
        current_lines: list[str] = []
        for line in body.splitlines():
            if line.startswith("## "):
                if current_sec:
                    sections[current_sec] = "\n".join(current_lines).strip()
                current_sec = line[3:].strip()
                current_lines = []
            elif current_sec:
                current_lines.append(line)
            elif not current_sec:
                current_lines.append(line)
        if current_sec:
            sections[current_sec] = "\n".join(current_lines).strip()
        # Extract decision checkboxes
        decisions: list[dict] = []
        for sec_name, sec_text in sections.items():
            for dline in sec_text.splitlines():
                dline = dline.strip()
                if dline.startswith("- [ ] ") or dline.startswith("- [x] "):
                    done = dline.startswith("- [x]")
                    decisions.append({
                        "text": dline[6:].strip(),
                        "done": done,
                        "section": sec_name,
                    })
        docs.append({
            "file": f.name,
            "meta": meta,
            "title": meta.get("title", f.stem.replace("_", " ").title()),
            "priority": meta.get("priority", "—"),
            "status": meta.get("status", "pending"),
            "sections": sections,
            "decisions": decisions,
            "body_md": body,
        })
    return docs


def next_feedback_id() -> int:
    """Get next feedback file number."""
    if not FEEDBACK_DIR.exists():
        return 1
    existing = list(FEEDBACK_DIR.glob("FEEDBACK_*.md"))
    nums = []
    for f in existing:
        m = re.search(r"FEEDBACK_(\d+)", f.stem)
        if m:
            nums.append(int(m.group(1)))
    return max(nums) + 1 if nums else 1


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


def _format_tokens(n: int) -> str:
    """Format token count: 1234 → 1.2K, 1234567 → 1.2M."""
    if n >= 1_000_000:
        return f"{n / 1_000_000:.1f}M"
    if n >= 1_000:
        return f"{n / 1_000:.1f}K"
    return str(n)


def _live_banner_html(
    hb: dict, is_paused: bool = False, rate: dict | None = None,
    tokens: dict | None = None, config: dict | None = None,
) -> str:
    """Generate live status banner HTML from heartbeat data."""
    runner_alive = _is_runner_alive()
    rate = rate or {}
    tokens = tokens or {}
    config = config or {}

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

    # Info bar: rate + tokens + account (shown below main banner)
    rate_pct = rate.get("pct", hb.get("rate_pct", 0))
    rate_color = "#3D9E8F" if rate_pct < 60 else "#C4A24D" if rate_pct < 80 else "#C75D4A"
    current_tok = _format_tokens(tokens.get("current_total", 0))
    total_tok = _format_tokens(tokens.get("total", 0))
    account = config.get("account_email", "—")
    model = config.get("model", hb.get("model", "—"))

    info_bar = (
        f'<div class="live-info">'
        f'<span class="live-info-item">Account: <strong>{account}</strong></span>'
        f'<span class="live-info-item">Model: <strong>{model}</strong></span>'
        f'<span class="live-info-item">Rate: <strong style="color:{rate_color}">{rate_pct:.1f}%</strong></span>'
        f'<span class="live-info-item">This cycle: <strong>{current_tok}</strong> tok</span>'
        f'<span class="live-info-item">All cycles: <strong>{total_tok}</strong> tok</span>'
        f'</div>'
    )

    if status == "running":
        return (
            f'<div class="live-banner live-running">'
            f'<span class="live-dot live-dot-running"></span>'
            f'<span class="live-label">Running cycle #{cycle}</span>'
            f'<span class="live-detail">{ctype} | {mode} mode | PID {pid}</span>'
            f'<span class="live-time">{elapsed}</span>'
            f'</div>{info_bar}'
        )
    elif status == "cooldown":
        cooldown_s = hb.get("cooldown_seconds", 300)
        return (
            f'<div class="live-banner live-cooldown">'
            f'<span class="live-dot live-dot-cooldown"></span>'
            f'<span class="live-label">Cooldown</span>'
            f'<span class="live-detail">After cycle #{cycle} | {cooldown_s}s pause</span>'
            f'<span class="live-time">{elapsed} ago</span>'
            f'</div>{info_bar}'
        )
    elif status == "completed":
        return (
            f'<div class="live-banner live-idle">'
            f'<span class="live-dot live-dot-idle"></span>'
            f'<span class="live-label">Completed cycle #{cycle}</span>'
            f'<span class="live-detail">{ctype} | awaiting next</span>'
            f'<span class="live-time">{elapsed} ago</span>'
            f'</div>{info_bar}'
        )
    elif status == "paused":
        return (
            f'<div class="live-banner live-paused">'
            f'<span class="live-dot live-dot-paused"></span>'
            f'<span class="live-label">Paused by operator</span>'
            f'<span class="live-detail">After cycle #{cycle} | waiting for resume</span>'
            f'<span class="live-time">{elapsed}</span>'
            f'</div>{info_bar}'
        )
    elif status in ("failed", "timeout"):
        exit_code = hb.get("exit_code", "?")
        return (
            f'<div class="live-banner live-failed">'
            f'<span class="live-dot live-dot-failed"></span>'
            f'<span class="live-label">{"Timed out" if status == "timeout" else "Failed"}: cycle #{cycle}</span>'
            f'<span class="live-detail">exit={exit_code}</span>'
            f'<span class="live-time">{elapsed} ago</span>'
            f'</div>{info_bar}'
        )
    else:
        if is_paused:
            label = "Agent paused"
            detail = "Paused by operator — press Start to resume"
        elif not runner_alive:
            label = "Agent stopped"
            detail = "Service not loaded — press Start to launch"
        else:
            label = "Agent idle"
            detail = "Runner alive, waiting for next cycle"
        return (
            f'<div class="live-banner live-idle">'
            f'<span class="live-dot live-dot-{"paused" if is_paused else "idle"}"></span>'
            f'<span class="live-label">{label}</span>'
            f'<span class="live-detail">{detail}</span>'
            f'<span class="live-time">{elapsed}</span>'
            f'</div>{info_bar}'
        )


def _is_runner_alive() -> bool:
    """Check if agent runner launchd service is loaded."""
    try:
        r = subprocess.run(
            ["launchctl", "list", AGENT_PLIST_LABEL],
            capture_output=True, text=True, timeout=5,
        )
        return r.returncode == 0
    except Exception:
        return False


def _agent_control_html(heartbeat_status: str) -> str:
    """Generate pause/start button HTML based on heartbeat status."""
    runner_alive = _is_runner_alive()
    # Show Pause when agent is actively running or in cooldown
    if runner_alive and heartbeat_status in ("running", "cooldown"):
        return (
            '<button class="agent-ctrl agent-ctrl-pause" id="agent-ctrl" onclick="agentControl(\'pause\')">'
            '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>'
            'Pause Agent'
            '</button>'
        )
    return (
        '<button class="agent-ctrl agent-ctrl-start" id="agent-ctrl" onclick="agentControl(\'start\')">'
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>'
        'Start Agent'
        '</button>'
    )


def _deliverables_html(reports: list[dict]) -> str:
    """Generate deliverables list from cycle reports."""
    if not reports:
        return '<p style="color:var(--c-muted);font-size:12px;">No cycle reports yet</p>'
    html = ""
    for r in reversed(reports):
        stars_html = " ".join(
            f'<span class="m-badge m-badge-{"value" if s == "V" else "elegance" if s == "E" else "reliability" if s == "R" else "awareness"}">{s}</span>'
            for s in r["north_stars"]
        )
        esc_count = len(r["escalations"])
        esc_tag = f'<span class="escalation-tag">{esc_count} escalation{"s" if esc_count != 1 else ""}</span>' if esc_count > 0 else ""
        # Changes section
        changes = r["sections"].get("Changes", "")
        changes_html = ""
        if changes:
            items = [l.strip()[2:] for l in changes.splitlines() if l.strip().startswith("- ")]
            if items:
                changes_items = "".join(f"<li>{_esc(i)}</li>" for i in items[:8])
                changes_html = f"<ul>{changes_items}</ul>"
                if len(items) > 8:
                    changes_html += f'<p style="font-size:11px;color:var(--c-muted);">+{len(items)-8} more</p>'
        # Impact
        impact_val = r["impact"]
        type_cls = {"META": "type-meta", "RESEARCH": "type-research", "ANALYSIS": "type-analysis",
                     "BUILD": "type-build", "ESCALATION": "type-escalation", "BOLD": "type-bold"}.get(r["type"], "")
        html += f"""<div class="deliverable">
          <div class="deliverable-header">
            <span class="deliverable-cycle">#{r["cycle"]}</span>
            <span class="type-pill {type_cls}">{r["type"]}</span>
            <span class="deliverable-title">{_esc(r["title"])}</span>
            {stars_html}{esc_tag}
            <span style="margin-left:auto;font-size:11px;color:var(--c-muted);">Impact: <strong>{impact_val}</strong></span>
          </div>
          <div class="deliverable-body">{changes_html}</div>
        </div>\n"""
    return html


def _escalations_html(reports: list[dict], esc_docs: list[dict] | None = None) -> str:
    """Generate expandable escalation cards with full context and decisions."""
    esc_docs = esc_docs or []
    # Collect inline escalations from cycle reports
    inline_esc: list[tuple[int, str, str]] = []
    for r in reports:
        for e in r["escalations"]:
            inline_esc.append((r["cycle"], r["title"], e))

    if not esc_docs and not inline_esc:
        return '<p style="color:var(--c-muted);font-size:12px;">No escalations pending</p>'

    html = ""
    # Render full escalation documents first (expandable)
    for idx, doc in enumerate(esc_docs):
        priority = doc["priority"]
        status = doc["status"]
        title = doc["title"]
        decisions = doc["decisions"]
        total_decisions = len(decisions)
        resolved = sum(1 for d in decisions if d["done"])
        pending = total_decisions - resolved

        priority_cls = "esc-p0" if priority == "P0" else "esc-p1" if priority == "P1" else ""
        status_cls = "esc-resolved" if status == "resolved" else "esc-pending"

        # Render sections as HTML
        body_html = ""
        for sec_name, sec_text in doc["sections"].items():
            sec_html = _md_to_html(sec_text)
            body_html += f'<div class="esc-section"><h4>{_esc(sec_name)}</h4>{sec_html}</div>'

        # Interactive decision items with approve/reject/auto + comment
        decisions_html = ""
        if decisions:
            decisions_html = '<div class="esc-decisions"><h4>Decisions Required</h4>'
            for di, d in enumerate(decisions):
                d_id = f"d-{idx}-{di}"
                decisions_html += f"""<div class="esc-decision" id="{d_id}">
                  <div class="esc-d-text">{_esc(d["text"])}</div>
                  <div class="esc-d-controls">
                    <button class="esc-d-btn" onclick="setDecision('{d_id}','yes',this)" title="Approve">Да</button>
                    <button class="esc-d-btn" onclick="setDecision('{d_id}','no',this)" title="Reject">Нет</button>
                    <button class="esc-d-btn" onclick="setDecision('{d_id}','auto',this)" title="Agent decides">Решай сам</button>
                    <input class="esc-d-comment" id="{d_id}-comment" placeholder="Комментарий..." />
                  </div>
                </div>\n"""
            decisions_html += "</div>"

        html += f"""<div class="esc-card" id="esc-doc-{idx}">
          <div class="esc-card-header" onclick="toggleEsc({idx})">
            <span class="esc-toggle" id="esc-toggle-{idx}">&#9654;</span>
            <span class="esc-priority {priority_cls}">{priority}</span>
            <span class="esc-card-title">{_esc(title)}</span>
            <span class="esc-status {status_cls}">{status}</span>
            <span class="esc-decision-count">{pending}/{total_decisions} decisions</span>
          </div>
          <div class="esc-card-body collapsed" id="esc-body-{idx}">
            {body_html}
            {decisions_html}
            <div class="esc-respond">
              <div class="esc-respond-note">General comment (optional):</div>
              <textarea class="esc-response-text" id="esc-text-{idx}" placeholder="Additional context or overall direction..."></textarea>
              <button class="fb-submit" onclick="submitEscalationDecisions({idx}, '{_esc(doc['file'])}', {total_decisions})">Submit All Decisions</button>
              <span class="esc-resp-status" id="esc-resp-{idx}"></span>
            </div>
          </div>
        </div>\n"""

    # Render inline escalations from cycle reports (not covered by docs)
    if inline_esc:
        html += '<div style="margin-top:12px;"><h4 style="font-size:12px;color:var(--c-muted);margin-bottom:8px;">From cycle reports</h4>'
        for cycle, title, esc in reversed(inline_esc):
            html += f'<div class="esc-inline"><span class="deliverable-cycle">#{cycle}</span> {_esc(esc)}</div>\n'
        html += "</div>"

    return html


def _md_to_html(md: str) -> str:
    """Minimal markdown to HTML for escalation docs."""
    lines = md.splitlines()
    html_parts: list[str] = []
    in_table = False
    in_list = False

    for line in lines:
        stripped = line.strip()
        if not stripped:
            if in_list:
                html_parts.append("</ul>")
                in_list = False
            if in_table:
                html_parts.append("</table>")
                in_table = False
            continue

        # Table rows
        if stripped.startswith("|") and stripped.endswith("|"):
            cells = [c.strip() for c in stripped.split("|")[1:-1]]
            if all(set(c) <= {"-", ":", " "} for c in cells):
                continue  # separator row
            if not in_table:
                html_parts.append('<table class="esc-table">')
                in_table = True
                html_parts.append("<tr>" + "".join(f"<th>{_esc(c)}</th>" for c in cells) + "</tr>")
            else:
                html_parts.append("<tr>" + "".join(f"<td>{_esc(c)}</td>" for c in cells) + "</tr>")
            continue

        if in_table:
            html_parts.append("</table>")
            in_table = False

        # Headings
        if stripped.startswith("### "):
            if in_list:
                html_parts.append("</ul>")
                in_list = False
            html_parts.append(f'<h5 style="margin:10px 0 4px;">{_esc(stripped[4:])}</h5>')
            continue

        # Checkboxes
        if stripped.startswith("- [ ] ") or stripped.startswith("- [x] "):
            if not in_list:
                html_parts.append("<ul class='esc-checklist'>")
                in_list = True
            done = stripped.startswith("- [x]")
            check_cls = "done" if done else ""
            text = stripped[6:]
            html_parts.append(f'<li class="{check_cls}">{_esc(text)}</li>')
            continue

        # List items
        if stripped.startswith("- "):
            if not in_list:
                html_parts.append("<ul>")
                in_list = True
            html_parts.append(f"<li>{_esc(stripped[2:])}</li>")
            continue

        if in_list:
            html_parts.append("</ul>")
            in_list = False

        # Blockquotes
        if stripped.startswith("> "):
            html_parts.append(f'<blockquote>{_esc(stripped[2:])}</blockquote>')
            continue

        # Bold text inline
        text = _esc(stripped)
        text = re.sub(r"\*\*(.+?)\*\*", r"<strong>\1</strong>", text)
        html_parts.append(f"<p>{text}</p>")

    if in_list:
        html_parts.append("</ul>")
    if in_table:
        html_parts.append("</table>")

    return "\n".join(html_parts)


def _feedback_form_html(reports: list[dict], feedback: list[dict]) -> str:
    """Generate feedback form."""
    # Cycle selector options
    cycle_opts = "".join(
        f'<option value="{r["cycle"]}">Cycle #{r["cycle"]}: {_esc(r["title"])}</option>'
        for r in reversed(reports)
    )
    if not cycle_opts:
        cycle_opts = '<option value="0">No cycles yet</option>'

    pending_count = sum(1 for fb in feedback if fb.get("meta", {}).get("status") == "pending")
    pending_note = f'<p style="font-size:11px;color:var(--m-value);margin-bottom:12px;">{pending_count} pending feedback item{"s" if pending_count != 1 else ""}</p>' if pending_count > 0 else ""

    return f"""
    {pending_note}
    <div class="feedback-form">
      <h3>Operator Feedback</h3>
      <form id="feedback-form" onsubmit="submitFeedback(event)">
        <div class="fb-row">
          <div class="fb-field">
            <label>Cycle</label>
            <select name="cycle_ref" id="fb-cycle">{cycle_opts}</select>
          </div>
          <div class="fb-field">
            <label>Category</label>
            <select name="category" id="fb-category">
              <option value="quality">Quality</option>
              <option value="methodology">Methodology</option>
              <option value="priority">Priority</option>
              <option value="domain-knowledge">Domain Knowledge</option>
            </select>
          </div>
        </div>
        <div class="fb-row">
          <div class="fb-field" style="flex:1;">
            <label>VERA Rating (optional)</label>
            <div class="fb-vera">
              <div class="fb-vera-item">
                <label>V</label>
                <select name="vera_v" id="fb-v"><option value="">—</option><option>1</option><option>2</option><option>3</option><option>4</option><option>5</option></select>
              </div>
              <div class="fb-vera-item">
                <label>E</label>
                <select name="vera_e" id="fb-e"><option value="">—</option><option>1</option><option>2</option><option>3</option><option>4</option><option>5</option></select>
              </div>
              <div class="fb-vera-item">
                <label>R</label>
                <select name="vera_r" id="fb-r"><option value="">—</option><option>1</option><option>2</option><option>3</option><option>4</option><option>5</option></select>
              </div>
              <div class="fb-vera-item">
                <label>A</label>
                <select name="vera_a" id="fb-a"><option value="">—</option><option>1</option><option>2</option><option>3</option><option>4</option><option>5</option></select>
              </div>
            </div>
          </div>
        </div>
        <div class="fb-row">
          <div class="fb-field" style="flex:1;">
            <label>Feedback (freeform)</label>
            <textarea name="text" id="fb-text" placeholder="What worked well? What needs improvement? Specific corrections or new priorities..."></textarea>
          </div>
        </div>
        <button type="submit" class="fb-submit" id="fb-submit">Submit Feedback</button>
        <div class="fb-success" id="fb-success">Feedback saved. Bot will process it in the next META cycle.</div>
        <div class="fb-error" id="fb-error"></div>
      </form>
    </div>
    """


def _esc(s: str) -> str:
    """Escape HTML."""
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def build_html(
    cycles: list[dict], state: dict, roadmap: dict, heartbeat: dict,
    is_paused: bool = False, rate: dict | None = None, tokens: dict | None = None,
    config: dict | None = None, reports: list[dict] | None = None,
    feedback: list[dict] | None = None, esc_docs: list[dict] | None = None,
) -> str:
    """Generate the full dashboard HTML."""
    now = datetime.now().strftime("%Y-%m-%d %H:%M")
    rate = rate or {}
    tokens = tokens or {}
    config = config or {}

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
.live-info {{ display:flex; gap:16px; padding:6px 20px; font-size:11px; color:var(--c-muted); flex-wrap:wrap; }}
.live-info-item {{ white-space:nowrap; }}
.live-info-item strong {{ color:var(--c-ink); font-weight:500; }}

/* Agent control button */
.agent-ctrl {{ display:inline-flex; align-items:center; gap:6px; padding:6px 16px; border-radius:var(--radius); border:1px solid var(--c-subtle); background:var(--c-card); color:var(--c-ink); font-size:12px; font-weight:500; cursor:pointer; transition:all 150ms; margin-left:12px; }}
.agent-ctrl:hover {{ background:var(--c-highlight); border-color:var(--c-muted); }}
.agent-ctrl-pause {{ border-color:rgba(199,93,74,0.4); }}
.agent-ctrl-pause:hover {{ background:rgba(199,93,74,0.08); }}
.agent-ctrl-start {{ border-color:rgba(61,158,143,0.4); }}
.agent-ctrl-start:hover {{ background:rgba(61,158,143,0.08); }}
.agent-ctrl:disabled {{ opacity:0.5; cursor:not-allowed; }}
.agent-ctrl svg {{ width:14px; height:14px; }}
.agent-ctrl svg.spin {{ animation:spin 1s linear infinite; }}
@keyframes spin {{ from {{ transform:rotate(0deg); }} to {{ transform:rotate(360deg); }} }}

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

/* Deliverables */
.deliverable {{ background:var(--c-card); border-radius:var(--radius); padding:14px 16px; margin-bottom:10px; }}
.deliverable-header {{ display:flex; align-items:center; gap:10px; margin-bottom:8px; }}
.deliverable-cycle {{ font-family:'JetBrains Mono',monospace; font-size:11px; background:var(--c-subtle); padding:2px 8px; border-radius:10px; }}
.deliverable-title {{ font-weight:500; font-size:13px; }}
.deliverable-body {{ font-size:12px; color:var(--c-muted); line-height:1.5; }}
.deliverable-body ul {{ padding-left:18px; margin:4px 0; }}
.deliverable-body li {{ margin-bottom:3px; }}
.escalation-list {{ margin-top:8px; padding:8px 12px; background:rgba(199,93,74,0.06); border-radius:var(--radius); border-left:3px solid #C75D4A; }}
.escalation-list li {{ color:var(--c-ink); font-size:12px; margin-bottom:4px; }}
.escalation-tag {{ display:inline-block; font-size:9px; font-weight:600; padding:2px 6px; border-radius:8px; background:rgba(199,93,74,0.15); color:#C75D4A; margin-left:6px; }}

/* Feedback form */
.feedback-form {{ background:var(--c-card); border-radius:var(--radius); padding:16px 20px; }}
.feedback-form h3 {{ font-family:'Newsreader',Georgia,serif; font-size:15px; margin-bottom:12px; }}
.fb-row {{ display:flex; gap:12px; margin-bottom:10px; flex-wrap:wrap; }}
.fb-field {{ display:flex; flex-direction:column; gap:4px; }}
.fb-field label {{ font-size:10px; color:var(--c-muted); text-transform:uppercase; letter-spacing:0.06em; }}
.fb-field select, .fb-field input {{ font-family:'Inter',sans-serif; font-size:12px; padding:6px 10px; border:1px solid var(--c-subtle); border-radius:var(--radius); background:var(--c-page); color:var(--c-ink); }}
.fb-field textarea {{ font-family:'Inter',sans-serif; font-size:12px; padding:8px 10px; border:1px solid var(--c-subtle); border-radius:var(--radius); background:var(--c-page); color:var(--c-ink); resize:vertical; min-height:80px; width:100%; }}
.fb-vera {{ display:flex; gap:10px; }}
.fb-vera-item {{ text-align:center; }}
.fb-vera-item label {{ font-size:10px; }}
.fb-vera-item select {{ width:52px; text-align:center; }}
.fb-submit {{ display:inline-flex; align-items:center; gap:6px; padding:8px 20px; border-radius:var(--radius); border:1px solid var(--m-elegance); background:rgba(61,158,143,0.08); color:var(--m-elegance-text); font-size:12px; font-weight:500; cursor:pointer; }}
.fb-submit:hover {{ background:rgba(61,158,143,0.15); }}
.fb-submit:disabled {{ opacity:0.5; cursor:not-allowed; }}
.fb-success {{ color:var(--m-elegance); font-size:12px; margin-top:8px; display:none; }}
.fb-error {{ color:#C75D4A; font-size:12px; margin-top:8px; display:none; }}

/* Escalation cards */
.esc-card {{ background:var(--c-card); border-radius:var(--radius); margin-bottom:10px; border-left:3px solid #C75D4A; }}
.esc-card-header {{ display:flex; align-items:center; gap:8px; padding:10px 14px; cursor:pointer; user-select:none; }}
.esc-card-header:hover {{ background:var(--c-highlight); border-radius:0 var(--radius) var(--radius) 0; }}
.esc-toggle {{ font-size:10px; color:var(--c-muted); transition:transform 150ms; flex-shrink:0; }}
.esc-toggle.open {{ transform:rotate(90deg); }}
.esc-priority {{ font-size:9px; font-weight:700; padding:2px 6px; border-radius:8px; flex-shrink:0; }}
.esc-p0 {{ background:rgba(199,93,74,0.15); color:#C75D4A; }}
.esc-p1 {{ background:rgba(196,162,77,0.15); color:#7A6420; }}
.esc-card-title {{ font-size:13px; font-weight:500; }}
.esc-status {{ font-size:10px; padding:2px 8px; border-radius:8px; margin-left:auto; flex-shrink:0; }}
.esc-pending {{ background:rgba(199,93,74,0.10); color:#C75D4A; }}
.esc-resolved {{ background:rgba(61,158,143,0.10); color:#3D9E8F; }}
.esc-decision-count {{ font-size:10px; color:var(--c-muted); flex-shrink:0; }}
.esc-card-body {{ padding:0 14px 14px; font-size:12px; line-height:1.6; }}
.esc-card-body.collapsed {{ display:none; }}
.esc-card-body h4 {{ font-family:'Newsreader',Georgia,serif; font-size:14px; margin:14px 0 6px; padding-top:10px; border-top:1px solid var(--c-subtle); }}
.esc-card-body h5 {{ font-size:12px; font-weight:600; color:var(--c-muted); }}
.esc-card-body p {{ margin:4px 0; }}
.esc-card-body ul {{ padding-left:16px; margin:4px 0; }}
.esc-card-body li {{ margin-bottom:3px; }}
.esc-card-body blockquote {{ border-left:3px solid var(--c-subtle); padding:4px 10px; margin:6px 0; color:var(--c-muted); font-style:italic; }}
.esc-table {{ width:100%; border-collapse:collapse; margin:8px 0; font-size:11px; }}
.esc-table th {{ text-align:left; padding:4px 8px; background:var(--c-subtle); font-weight:500; font-size:10px; text-transform:uppercase; letter-spacing:0.04em; }}
.esc-table td {{ padding:4px 8px; border-bottom:1px solid var(--c-subtle); }}
.esc-checklist {{ list-style:none; padding-left:0; }}
.esc-checklist li {{ padding:2px 0 2px 20px; position:relative; }}
.esc-checklist li::before {{ content:'\\2610'; position:absolute; left:0; }}
.esc-checklist li.done {{ color:var(--c-muted); text-decoration:line-through; }}
.esc-checklist li.done::before {{ content:'\\2611'; color:var(--m-elegance); }}
.esc-decisions {{ margin-top:12px; padding:12px; background:rgba(199,93,74,0.04); border-radius:var(--radius); }}
.esc-decisions h4 {{ margin:0 0 10px !important; padding:0 !important; border:none !important; font-size:12px !important; color:#C75D4A; }}
.esc-decision {{ padding:8px 0; border-bottom:1px solid var(--c-subtle); }}
.esc-decision:last-child {{ border-bottom:none; }}
.esc-d-text {{ font-size:12px; margin-bottom:6px; line-height:1.4; }}
.esc-d-controls {{ display:flex; align-items:center; gap:6px; flex-wrap:wrap; }}
.esc-d-btn {{ font-size:10px; padding:3px 10px; border-radius:10px; border:1px solid var(--c-subtle); background:none; color:var(--c-muted); cursor:pointer; transition:all 100ms; }}
.esc-d-btn:hover {{ border-color:var(--c-muted); }}
.esc-d-btn.selected {{ font-weight:600; }}
.esc-d-btn.sel-yes {{ background:rgba(61,158,143,0.15); border-color:var(--m-elegance); color:var(--m-elegance-text); }}
.esc-d-btn.sel-no {{ background:rgba(199,93,74,0.12); border-color:#C75D4A; color:#C75D4A; }}
.esc-d-btn.sel-auto {{ background:rgba(139,126,200,0.12); border-color:var(--m-awareness); color:var(--m-awareness-text); }}
.esc-d-comment {{ flex:1; min-width:180px; font-family:'Inter',sans-serif; font-size:11px; padding:4px 8px; border:1px solid var(--c-subtle); border-radius:var(--radius); background:var(--c-page); color:var(--c-ink); }}
.esc-d-comment:focus {{ border-color:var(--m-elegance); outline:none; }}
.esc-d-comment::placeholder {{ color:var(--c-muted); }}
.esc-respond {{ margin-top:14px; padding-top:12px; border-top:1px solid var(--c-subtle); }}
.esc-respond-note {{ font-size:11px; color:var(--c-muted); margin-bottom:8px; }}
.esc-response-text {{ width:100%; min-height:50px; padding:8px 10px; border:1px solid var(--c-subtle); border-radius:var(--radius); font-family:'Inter',sans-serif; font-size:12px; background:var(--c-page); color:var(--c-ink); resize:vertical; margin-bottom:8px; }}
.esc-resp-status {{ font-size:11px; margin-left:8px; }}
.esc-inline {{ display:flex; align-items:center; gap:8px; padding:4px 0; font-size:12px; }}

/* Tabs */
.tab-bar {{ display:flex; gap:2px; margin-bottom:16px; border-bottom:1px solid var(--c-subtle); }}
.tab-btn {{ padding:8px 16px; font-size:12px; font-weight:500; border:none; background:none; color:var(--c-muted); cursor:pointer; border-bottom:2px solid transparent; }}
.tab-btn:hover {{ color:var(--c-ink); }}
.tab-btn.active {{ color:var(--c-ink); border-bottom-color:var(--m-elegance); }}
.tab-panel {{ display:none; }}
.tab-panel.active {{ display:block; }}
</style>
</head>
<body>

<div style="display:flex;align-items:baseline;gap:12px;">
<h1>Plot Bot</h1>
{_agent_control_html(heartbeat.get("status", "unknown"))}
</div>
<p class="subtitle">Cycle Dashboard — generated {now}</p>

<!-- Live Status -->
{_live_banner_html(heartbeat, is_paused=is_paused, rate=rate, tokens=tokens, config=config)}

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

<!-- Deliverables & Feedback -->
<div class="card" style="margin-bottom:16px;">
  <div class="card-title">Deliverables & Feedback</div>
  <div class="tab-bar">
    <button class="tab-btn active" onclick="switchTab('deliverables')">Deliverables</button>
    <button class="tab-btn" onclick="switchTab('escalations')">Escalations</button>
    <button class="tab-btn" onclick="switchTab('feedback')">Give Feedback</button>
  </div>

  <div class="tab-panel active" id="tab-deliverables">
    {_deliverables_html(reports or [])}
  </div>

  <div class="tab-panel" id="tab-escalations">
    {_escalations_html(reports or [], esc_docs or [])}
  </div>

  <div class="tab-panel" id="tab-feedback">
    {_feedback_form_html(reports or [], feedback or [])}
  </div>
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

// Agent control: start / pause with visual feedback + polling
async function agentControl(action) {{
  const btn = document.getElementById('agent-ctrl');
  const banner = document.querySelector('.live-banner');
  if (!btn) return;

  // Save original button content
  const origHTML = btn.innerHTML;
  btn.disabled = true;

  // Show loading state
  if (action === 'start') {{
    btn.innerHTML = '<svg class="spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4m0 12v4m-7.07-3.93l2.83-2.83m8.48-8.48l2.83-2.83M2 12h4m12 0h4m-3.93 7.07l-2.83-2.83M7.76 7.76L4.93 4.93"/></svg>Starting agent...';
    if (banner) {{
      banner.className = 'live-banner live-cooldown';
      banner.innerHTML = '<span class="live-dot live-dot-cooldown"></span><span class="live-label">Starting agent...</span><span class="live-detail">Loading launchd service</span><span class="live-time"></span>';
    }}
  }} else {{
    btn.innerHTML = '<svg class="spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4m0 12v4m-7.07-3.93l2.83-2.83m8.48-8.48l2.83-2.83M2 12h4m12 0h4m-3.93 7.07l-2.83-2.83M7.76 7.76L4.93 4.93"/></svg>Pausing...';
    if (banner) {{
      banner.innerHTML = '<span class="live-dot live-dot-cooldown"></span><span class="live-label">Pausing agent...</span><span class="live-detail">Current cycle will finish first</span><span class="live-time"></span>';
    }}
  }}

  try {{
    const r = await fetch('/api/agent/' + action, {{ method: 'POST' }});
    const d = await r.json();
    if (!r.ok) {{
      showError(d.error || 'Unknown error');
      btn.innerHTML = origHTML;
      btn.disabled = false;
      return;
    }}

    // Poll until status changes or timeout (30s for start, 5s for pause)
    const maxWait = action === 'start' ? 30000 : 5000;
    const started = Date.now();
    const targetStatus = action === 'start' ? 'running' : 'paused';

    while (Date.now() - started < maxWait) {{
      await new Promise(ok => setTimeout(ok, 1500));
      try {{
        const sr = await fetch('/api/status');
        const sd = await sr.json();
        if (action === 'start' && sd.runner_alive) {{
          // Runner is up — success even if first heartbeat not yet written
          if (banner) {{
            banner.className = 'live-banner live-running';
            banner.innerHTML = '<span class="live-dot live-dot-running"></span><span class="live-label">Agent started</span><span class="live-detail">Reloading...</span><span class="live-time"></span>';
          }}
          setTimeout(() => location.reload(), 800);
          return;
        }}
        if (action === 'pause' && sd.paused) {{
          location.reload();
          return;
        }}
      }} catch(e) {{ /* ignore poll errors */ }}
    }}

    // Timeout — reload anyway to show current state
    location.reload();

  }} catch(e) {{
    showError('Dashboard not in serve mode — control unavailable');
    btn.innerHTML = origHTML;
    btn.disabled = false;
  }}
}}

// Escalation toggle
function toggleEsc(idx) {{
  const body = document.getElementById('esc-body-' + idx);
  const toggle = document.getElementById('esc-toggle-' + idx);
  if (!body || !toggle) return;
  body.classList.toggle('collapsed');
  toggle.classList.toggle('open');
}}

// Per-decision vote button
function setDecision(dId, vote, btn) {{
  // Remove selected from siblings
  btn.parentElement.querySelectorAll('.esc-d-btn').forEach(b => {{
    b.classList.remove('selected', 'sel-yes', 'sel-no', 'sel-auto');
  }});
  btn.classList.add('selected', 'sel-' + vote);
  btn.dataset.vote = vote;
}}

// Collect all decisions and submit as structured feedback
async function submitEscalationDecisions(escIdx, file, totalDecisions) {{
  const statusEl = document.getElementById('esc-resp-' + escIdx);
  const generalText = document.getElementById('esc-text-' + escIdx).value.trim();

  // Collect per-decision votes + comments
  const decisions = [];
  let hasAnyInput = false;
  for (let i = 0; i < totalDecisions; i++) {{
    const dId = 'd-' + escIdx + '-' + i;
    const el = document.getElementById(dId);
    if (!el) continue;

    const textEl = el.querySelector('.esc-d-text');
    const question = textEl ? textEl.textContent.trim() : '';
    const selectedBtn = el.querySelector('.esc-d-btn.selected');
    const vote = selectedBtn ? selectedBtn.dataset.vote : '';
    const comment = document.getElementById(dId + '-comment').value.trim();

    if (vote || comment) hasAnyInput = true;
    decisions.push({{ question, vote, comment }});
  }}

  if (!hasAnyInput && !generalText) {{
    statusEl.textContent = 'Mark at least one decision or write a comment.';
    statusEl.style.color = '#C75D4A';
    return;
  }}

  statusEl.textContent = 'Saving...';
  statusEl.style.color = 'var(--c-muted)';

  // Format as structured feedback text
  let feedbackText = 'ESCALATION RESPONSE: ' + file + '\\n\\n';
  decisions.forEach((d, i) => {{
    if (!d.vote && !d.comment) return;
    const voteLabel = d.vote === 'yes' ? 'ДА' : d.vote === 'no' ? 'НЕТ' : d.vote === 'auto' ? 'РЕШАЙ САМ' : '—';
    feedbackText += '**Q' + (i+1) + ':** ' + d.question + '\\n';
    feedbackText += '**Decision:** ' + voteLabel;
    if (d.comment) feedbackText += ' — ' + d.comment;
    feedbackText += '\\n\\n';
  }});
  if (generalText) {{
    feedbackText += '**General:** ' + generalText + '\\n';
  }}

  try {{
    const r = await fetch('/api/feedback', {{
      method: 'POST',
      headers: {{ 'Content-Type': 'application/json' }},
      body: JSON.stringify({{
        cycle_ref: '0',
        category: 'priority',
        text: feedbackText,
        vera: {{}},
      }}),
    }});
    if (r.ok) {{
      const d = await r.json();
      const answered = decisions.filter(x => x.vote || x.comment).length;
      statusEl.textContent = answered + '/' + totalDecisions + ' decisions saved as ' + (d.file || 'feedback') + '. Bot processes in next META.';
      statusEl.style.color = 'var(--m-elegance)';
    }} else {{
      const d = await r.json();
      statusEl.textContent = d.error || 'Failed';
      statusEl.style.color = '#C75D4A';
    }}
  }} catch(e) {{
    statusEl.textContent = 'Dashboard not in serve mode';
    statusEl.style.color = '#C75D4A';
  }}
}}

// Tab switching
function switchTab(tab) {{
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
  event.target.classList.add('active');
  const panel = document.getElementById('tab-' + tab);
  if (panel) panel.classList.add('active');
}}

// Feedback form submission
async function submitFeedback(e) {{
  e.preventDefault();
  const btn = document.getElementById('fb-submit');
  const successEl = document.getElementById('fb-success');
  const errorEl = document.getElementById('fb-error');
  successEl.style.display = 'none';
  errorEl.style.display = 'none';
  btn.disabled = true;
  btn.textContent = 'Saving...';

  const data = {{
    cycle_ref: document.getElementById('fb-cycle').value,
    category: document.getElementById('fb-category').value,
    text: document.getElementById('fb-text').value,
    vera: {{
      V: document.getElementById('fb-v').value || null,
      E: document.getElementById('fb-e').value || null,
      R: document.getElementById('fb-r').value || null,
      A: document.getElementById('fb-a').value || null,
    }},
  }};

  if (!data.text.trim()) {{
    errorEl.textContent = 'Please enter feedback text.';
    errorEl.style.display = 'block';
    btn.disabled = false;
    btn.textContent = 'Submit Feedback';
    return;
  }}

  try {{
    const r = await fetch('/api/feedback', {{
      method: 'POST',
      headers: {{ 'Content-Type': 'application/json' }},
      body: JSON.stringify(data),
    }});
    const d = await r.json();
    if (r.ok) {{
      successEl.textContent = 'Feedback saved as ' + (d.file || 'FEEDBACK_XXX.md') + '. Bot will process it in the next META cycle.';
      successEl.style.display = 'block';
      document.getElementById('fb-text').value = '';
      document.getElementById('fb-v').value = '';
      document.getElementById('fb-e').value = '';
      document.getElementById('fb-r').value = '';
      document.getElementById('fb-a').value = '';
    }} else {{
      errorEl.textContent = d.error || 'Failed to save feedback';
      errorEl.style.display = 'block';
    }}
  }} catch(err) {{
    errorEl.textContent = 'Dashboard not in serve mode — feedback unavailable';
    errorEl.style.display = 'block';
  }}
  btn.disabled = false;
  btn.textContent = 'Submit Feedback';
}}

function showError(msg) {{
  const banner = document.querySelector('.live-banner');
  if (banner) {{
    banner.className = 'live-banner live-failed';
    banner.innerHTML = '<span class="live-dot live-dot-failed"></span><span class="live-label">Error</span><span class="live-detail">' + msg + '</span><span class="live-time"></span>';
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
    rate = parse_rate_control()
    tokens = parse_session_tokens()
    config = parse_agent_config()
    reports = parse_cycle_reports()
    fb = parse_feedback()
    esc_docs = parse_escalation_docs()

    html = build_html(cycles, state, roadmap, heartbeat, is_paused=paused, rate=rate,
                      tokens=tokens, config=config, reports=reports, feedback=fb, esc_docs=esc_docs)
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
                # Check if runner process is alive via launchctl
                runner_alive = False
                try:
                    r = subprocess.run(
                        ["launchctl", "list", AGENT_PLIST_LABEL],
                        capture_output=True, text=True, timeout=5,
                    )
                    runner_alive = r.returncode == 0
                except Exception:
                    pass
                data = {**hb, "state": st, "paused": PAUSE_FILE.exists(), "runner_alive": runner_alive}
                self._json_response(200, data)
                return

            if path == "/api/deliverables":
                reports = parse_cycle_reports()
                fb = parse_feedback()
                self._json_response(200, {"reports": reports, "feedback": fb})
                return

            if path == "/" or path == "/dashboard":
                # Rebuild on every request for live data
                cycles = parse_progress()
                state = parse_state()
                heartbeat = parse_heartbeat()
                roadmap = parse_roadmap()
                paused = PAUSE_FILE.exists()
                rate = parse_rate_control()
                tokens = parse_session_tokens()
                config = parse_agent_config()
                reports = parse_cycle_reports()
                fb = parse_feedback()
                esc_docs = parse_escalation_docs()
                html = build_html(cycles, state, roadmap, heartbeat, is_paused=paused, rate=rate,
                                  tokens=tokens, config=config, reports=reports, feedback=fb, esc_docs=esc_docs)
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
                # Soft pause: write signal file, runner finishes current cycle then waits
                PAUSE_FILE.write_text(
                    f'{{"paused_at": "{datetime.now().isoformat()}", "by": "dashboard"}}\n'
                )
                self._json_response(200, {"ok": True, "paused": True,
                    "message": "Pause signal set. Current cycle will finish, then agent pauses."})
                return

            if path == "/api/agent/start":
                # Remove pause file first
                if PAUSE_FILE.exists():
                    PAUSE_FILE.unlink()

                runner_alive = False
                try:
                    r = subprocess.run(
                        ["launchctl", "list", AGENT_PLIST_LABEL],
                        capture_output=True, text=True, timeout=5,
                    )
                    runner_alive = r.returncode == 0
                except Exception:
                    pass

                if runner_alive:
                    # Runner is already loaded — just needed the pause file removed
                    self._json_response(200, {"ok": True, "paused": False,
                        "message": "Pause cleared. Runner already active."})
                    return

                # Runner not loaded — need to load it via launchctl
                if not AGENT_PLIST_PATH.exists():
                    src = REPO / "tools" / "scripts" / f"{AGENT_PLIST_LABEL}.plist"
                    if src.exists():
                        import shutil
                        shutil.copy2(str(src), str(AGENT_PLIST_PATH))
                    else:
                        self._json_response(500, {"error": f"Plist not found: {src}"})
                        return

                # Unload (idempotent), then load
                subprocess.run(
                    ["launchctl", "unload", str(AGENT_PLIST_PATH)],
                    capture_output=True, timeout=10,
                )
                result = subprocess.run(
                    ["launchctl", "load", str(AGENT_PLIST_PATH)],
                    capture_output=True, text=True, timeout=10,
                )
                if result.returncode != 0:
                    self._json_response(500, {
                        "error": f"launchctl load failed: {result.stderr.strip()}",
                    })
                    return
                self._json_response(200, {"ok": True, "paused": False,
                    "message": "Agent service loaded and started."})
                return

            if path == "/api/feedback":
                content_length = int(self.headers.get("Content-Length", 0))
                body = self.rfile.read(content_length).decode() if content_length > 0 else "{}"
                try:
                    data = json.loads(body)
                except json.JSONDecodeError:
                    self._json_response(400, {"error": "Invalid JSON"})
                    return

                text = data.get("text", "").strip()
                if not text:
                    self._json_response(400, {"error": "Feedback text is required"})
                    return

                FEEDBACK_DIR.mkdir(parents=True, exist_ok=True)
                fb_id = next_feedback_id()
                cycle_ref = data.get("cycle_ref", "0")
                category = data.get("category", "quality")
                vera = data.get("vera", {})
                today = datetime.now().strftime("%Y-%m-%d")

                vera_table = ""
                for axis in ["V", "E", "R", "A"]:
                    v = vera.get(axis, "")
                    if v:
                        axis_name = {"V": "Value", "E": "Elegance", "R": "Reliability", "A": "Awareness"}[axis]
                        vera_table += f"| {axis} ({axis_name}) | {v} | |\n"

                content = f"""---
id: feedback-{fb_id:03d}
type: insight
status: pending
category: {category}
source: dashboard
created: {today}
cycle_ref: cycle-report-{cycle_ref:0>3}
---

## Feedback

{text}
"""
                if vera_table:
                    content += f"""
## VERA Rating

| Axis | Score (1-5) | Comment |
|------|-------------|---------|
{vera_table}
"""
                content += """
## Action Required

- [ ] Process this feedback

## Resolution

Status: pending
"""
                fname = f"FEEDBACK_{fb_id:03d}.md"
                (FEEDBACK_DIR / fname).write_text(content)
                self._json_response(200, {"ok": True, "file": fname, "id": fb_id})
                return

            if path == "/api/agent/stop":
                # Hard stop: unload launchd service (kills runner mid-cycle if running)
                PAUSE_FILE.write_text(
                    f'{{"paused_at": "{datetime.now().isoformat()}", "by": "dashboard", "hard_stop": true}}\n'
                )
                result = subprocess.run(
                    ["launchctl", "unload", str(AGENT_PLIST_PATH)],
                    capture_output=True, text=True, timeout=10,
                )
                self._json_response(200, {"ok": True, "stopped": True,
                    "message": "Agent service unloaded."})
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
