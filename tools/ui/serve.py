#!/usr/bin/env python3
"""AWR dashboard server: static files + API proxy to ./ask + data aggregation.

- Clean URLs: /navigator → navigator.html
- Root redirects to /navigator
- Non-static files (like this script) return 404
- Directory listings disabled

Modules in api/ handle each endpoint group:
  ask_proxy  — ./ask CLI proxy, artifact index, entity registry
  xray       — session analytics, turn detail, reports
  usage      — Anthropic OAuth usage (3-tier cache)
  strategy   — bets, maintenance, domain stats, activity feed
  intake     — meeting file management
  deeplinks  — Jira, Fireflies, artifact, person redirects
  queue      — work queue, value chains
"""

import http.server
import os
import re
import sys

# Ensure api/ package is importable
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from api import serve_json, json_error, redirect
from api import ask_proxy, xray, usage, strategy, intake, deeplinks, queue, rate

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 3000
ROOT = os.path.dirname(os.path.abspath(__file__))
STATIC_EXT = {".html", ".css", ".js", ".json", ".svg", ".png", ".ico", ".woff2"}

# Route table: (regex_pattern, handler_function)
# Order matters — more specific patterns first.
ROUTES = [
    (r"^/api/usage$", usage.anthropic_usage),
    (r"^/api/xray/sessions$", xray.sessions_list),
    (r"^/api/xray/reports-index$", xray.reports_index),
    (r"^/api/xray/", xray.proxy_xray),
    (r"^/api/domains/tree$", strategy.domain_tree),
    (r"^/api/strategy$", strategy.strategy_data),
    (r"^/api/activity$", strategy.activity_feed),
    (r"^/api/rate$", rate.rate_data),
    (r"^/api/ask/rate$", rate.rate_data),  # legacy URL
    (r"^/api/ask/", ask_proxy.proxy_ask),
    (r"^/api/index$", ask_proxy.serve_index),
    (r"^/api/entities/registry$", ask_proxy.entity_registry),
    (r"^/api/queue$", queue.queue_api),
    (r"^/api/intake", intake.intake_api),
    (r"^/api/chains$", queue.chains_data),
]

POST_ROUTES = [
    (r"^/api/queue/approve$", queue.queue_approve),
]

DEEPLINK_ROUTES = [
    ("/j/", deeplinks.jira),
    ("/f/", deeplinks.fireflies),
    ("/a/", deeplinks.artifact),
    ("/p/", deeplinks.person),
]

# Compile route patterns once
_COMPILED_ROUTES = [(re.compile(p), fn) for p, fn in ROUTES]
_COMPILED_POST_ROUTES = [(re.compile(p), fn) for p, fn in POST_ROUTES]


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *a, **kw):
        super().__init__(*a, directory=ROOT, **kw)

    def do_GET(self):
        path_clean = self.path.split("?")[0].rstrip("/") or "/"

        # API routes (dispatch table)
        for pattern, handler_fn in _COMPILED_ROUTES:
            if pattern.match(path_clean):
                return handler_fn(self)

        # Deep link routes
        for prefix, handler_fn in DEEPLINK_ROUTES:
            if path_clean.startswith(prefix):
                return handler_fn(self)

        # Root → redirect to /navigator
        if path_clean == "/":
            return redirect(self, "/navigator")

        ext = os.path.splitext(path_clean)[1].lower()

        # Clean URL: /navigator → serve navigator.html
        if not ext:
            html_path = path_clean + ".html"
            full = os.path.join(ROOT, html_path.lstrip("/"))
            if os.path.isfile(full):
                self.path = html_path
                return super().do_GET()
            self.send_response(404)
            self.end_headers()
            return

        # Block non-static files
        if ext not in STATIC_EXT:
            self.send_response(404)
            self.end_headers()
            return

        return super().do_GET()

    def do_POST(self):
        path_clean = self.path.split("?")[0].rstrip("/")
        for pattern, handler_fn in _COMPILED_POST_ROUTES:
            if pattern.match(path_clean):
                return handler_fn(self)
        self.send_response(404)
        self.end_headers()

    def do_HEAD(self):
        """Route HEAD through do_GET for deep links."""
        self.do_GET()

    def list_directory(self, path):
        self.send_response(404)
        self.end_headers()
        return None

    def log_message(self, fmt, *a):
        sys.stderr.write(f"[serve] {fmt % a}\n")


if __name__ == "__main__":
    print(f"Serving on http://localhost:{PORT}")
    http.server.ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
