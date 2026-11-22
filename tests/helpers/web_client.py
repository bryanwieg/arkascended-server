"""HTTP client helper for web integration tests.

Uses stdlib urllib so no extra dependencies are required.
Logs requests and responses to the commands log.
"""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass
class WebResponse:
    status_code: int
    text: str
    content: bytes

    def json(self) -> Any:
        return json.loads(self.text)


class WebClient:
    """Thin wrapper around urllib that logs to commands_log."""

    def __init__(self, base_url: str, commands_log: str | None = None):
        self.base_url = base_url.rstrip("/")
        self._log_path = Path(commands_log) if commands_log else None

    def _log(self, method: str, path: str, status: int, body: str) -> None:
        if not self._log_path:
            return
        with self._log_path.open("a", encoding="utf-8") as f:
            f.write(f"\n===== {method} {path} (HTTP {status}) =====\n")
            f.write(body)
            f.write("\n")

    def _url(self, path: str) -> str:
        return f"{self.base_url}{path}"

    def _request(self, method: str, path: str, data: bytes | None = None) -> WebResponse:
        url = self._url(path)
        req = urllib.request.Request(url, data=data, method=method)  # noqa: S310
        if data:
            req.add_header("Content-Type", "application/x-www-form-urlencoded")
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:  # noqa: S310
                body = resp.read()
                text = body.decode("utf-8", errors="replace")
                self._log(method, path, resp.status, text)
                return WebResponse(status_code=resp.status, text=text, content=body)
        except urllib.error.HTTPError as e:
            body = e.read()
            text = body.decode("utf-8", errors="replace")
            self._log(method, path, e.code, text)
            return WebResponse(status_code=e.code, text=text, content=body)

    def get(self, path: str) -> WebResponse:
        return self._request("GET", path)

    def post(self, path: str, data: dict[str, str] | None = None) -> WebResponse:
        body = urllib.parse.urlencode(data).encode() if data else None
        return self._request("POST", path, data=body)

    def close(self) -> None:
        pass


def wait_for_web_ready(web_client: WebClient, timeout: int = 30) -> None:
    """Poll /health until the web server responds 200."""
    import time

    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            resp = web_client.get("/health")
            if resp.status_code == 200:
                return
        except Exception:  # noqa: BLE001, S110
            pass
        time.sleep(0.5)
    raise RuntimeError(f"Web server not ready after {timeout}s")


def daemon_host(env: dict[str, str]) -> str:
    """Host where the engine publishes ports: the tcp:// DOCKER_HOST's host, else this machine."""
    docker_host = env.get("DOCKER_HOST", "")
    return urllib.parse.urlparse(docker_host).hostname if docker_host.startswith("tcp://") else "127.0.0.1"


def scheduler_health(env: dict[str, str], port: int) -> tuple[int | None, dict | None]:
    """GET the scheduler's /health on a published port: (status, report), report None on HTTP errors.

    (None, None) when nothing answers.
    """
    try:
        with urllib.request.urlopen(f"http://{daemon_host(env)}:{port}/health", timeout=5) as resp:
            return resp.status, json.load(resp)
    except urllib.error.HTTPError as exc:
        return exc.code, None
    except OSError:
        return None, None
