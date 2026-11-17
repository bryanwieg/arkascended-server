"""Fake HTTP servers for integration tests.

Context managers that start local HTTP servers mimicking external APIs
(Steam API, Docker Hub) so tests can exercise CLI behavior without
real network calls.
"""

from __future__ import annotations

import json
import socketserver
import threading
from collections.abc import Iterator
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler


@contextmanager
def steam_api_server(*, build_id: str) -> Iterator[str]:
    """Start a local HTTP server that mimics the steamcmd.net API for app 2430930.

    Yields the URL to use as ARK_STEAM_API_URL.
    """
    response_body = json.dumps(
        {
            "data": {
                "2430930": {
                    "depots": {
                        "branches": {
                            "public": {
                                "buildid": build_id,
                            },
                        },
                    },
                },
            },
        }
    ).encode("utf-8")

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            if "/v1/info/2430930" in self.path:
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response_body)))
                self.end_headers()
                self.wfile.write(response_body)
                return
            self.send_response(404)
            self.end_headers()

        def log_message(self, _format: str, *args: object) -> None:
            return

    with socketserver.TCPServer(("127.0.0.1", 0), Handler) as server:
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            port = server.server_address[1]
            yield f"http://127.0.0.1:{port}/v1/info/2430930"
        finally:
            server.shutdown()
            thread.join(timeout=5)
