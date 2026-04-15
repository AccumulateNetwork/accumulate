#!/usr/bin/env python3
"""Dashboard server. Serves dashboard.html, status.json, and log.jsonl."""

import json
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path

WORKSPACE = Path('/tmp/loadtest-workspace')
STATUS_FILE = WORKSPACE / 'status.json'
LOG_FILE = WORKSPACE / 'log.jsonl'
DASHBOARD_FILE = Path(__file__).parent / 'dashboard.html'
FALLBACK = '{"stale":true,"test_label":"","current_tps":0,"target_tps":0,"peak_tps":0,"submitted":0,"succeeded":0,"failed":0,"error_rate":0,"avg_tps":0,"elapsed":"--","workers":0,"nodes":0}'


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/status':
            body = FALLBACK
            try:
                mtime = STATUS_FILE.stat().st_mtime
                raw = STATUS_FILE.read_text()
                data = json.loads(raw)
                if time.time() - mtime > 5:
                    data['stale'] = True
                body = json.dumps(data)
            except Exception:
                pass

            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(body.encode())

        elif self.path == '/log':
            body = '[]'
            try:
                lines = LOG_FILE.read_text().strip().split('\n')
                # Return last 200 entries
                lines = lines[-200:]
                body = json.dumps([json.loads(l) for l in lines if l.strip()])
            except Exception:
                pass

            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(body.encode())

        elif self.path == '/' or self.path == '/dashboard':
            try:
                content = DASHBOARD_FILE.read_bytes()
                self.send_response(200)
                self.send_header('Content-Type', 'text/html')
                self.end_headers()
                self.wfile.write(content)
            except FileNotFoundError:
                self.send_error(404, 'dashboard.html not found')

        else:
            self.send_error(404)

    def log_message(self, format, *args):
        pass


if __name__ == '__main__':
    import sys
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8888
    server = HTTPServer(('0.0.0.0', port), Handler)
    print(f"Dashboard server on http://localhost:{port}/")
    server.serve_forever()
