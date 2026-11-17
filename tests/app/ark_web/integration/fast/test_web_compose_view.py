"""Fast integration tests for the web compose editor."""

from __future__ import annotations

import uuid
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose import write_single_service_compose

if TYPE_CHECKING:
    from helpers.web_client import WebClient

pytestmark = [pytest.mark.container]


def test_web_health_endpoint(
    start_web_server: tuple[WebClient, int],
) -> None:
    """Health endpoint responds 200 with status ok."""
    web, _port = start_web_server
    resp = web.get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"


def test_home_page_renders(
    start_web_server: tuple[WebClient, int],
) -> None:
    """Home page renders HTML with expected elements."""
    web, _port = start_web_server
    resp = web.get("/")
    assert resp.status_code == 200
    assert "<title>ARK Manager</title>" in resp.text
    assert "ARK Manager" in resp.text


def test_instances_page_renders(
    start_web_server: tuple[WebClient, int],
) -> None:
    """Instances page renders with table structure."""
    web, _port = start_web_server
    resp = web.get("/instances")
    assert resp.status_code == 200
    assert "<title>Instances" in resp.text


def test_instance_detail_page(
    start_web_server: tuple[WebClient, int],
) -> None:
    """Instance detail page renders for a known service."""
    web, _port = start_web_server
    resp = web.get("/instances/asa1")
    assert resp.status_code == 200
    assert "asa1" in resp.text


def test_compose_editor_loads(
    start_web_server: tuple[WebClient, int],
    web_test_env: dict[str, str],
) -> None:
    """Compose editor loads the compose YAML content."""
    web, _port = start_web_server

    instance = f"pytest-web-{uuid.uuid4().hex[:8]}"
    compose_file = Path(web_test_env["COMPOSE_FILE"])
    write_single_service_compose(
        compose_file,
        instance=instance,
        image="ark-server:latest",
        admin_password="AdminPass123",
        server_password="",
        entrypoint='["sh", "-c", "echo ready && sleep 30"]',
    )

    resp = web.get(f"/instances/{instance}/compose")
    assert resp.status_code == 200
    assert instance in resp.text
    assert "Save" in resp.text
    assert "textarea" in resp.text
    assert "services:" in resp.text


def test_compose_editor_save(
    start_web_server: tuple[WebClient, int],
    web_test_env: dict[str, str],
) -> None:
    """Compose editor saves valid YAML and persists to disk."""
    web, _port = start_web_server

    instance = f"pytest-web-{uuid.uuid4().hex[:8]}"
    compose_file = Path(web_test_env["COMPOSE_FILE"])
    write_single_service_compose(
        compose_file,
        instance=instance,
        image="ark-server:latest",
        admin_password="AdminPass123",
        server_password="",
        entrypoint='["sh", "-c", "echo ready && sleep 30"]',
    )

    # Read current content
    original = compose_file.read_text()

    # Modify it trivially (add a comment)
    new_content = original + "# test comment\n"

    resp = web.post(
        f"/instances/{instance}/compose",
        data={"content": new_content},
    )
    assert resp.status_code == 200
    assert "Saved successfully" in resp.text

    # Verify the file was actually written
    written = compose_file.read_text()
    assert written == new_content


def test_compose_editor_rejects_invalid_yaml(
    start_web_server: tuple[WebClient, int],
    web_test_env: dict[str, str],
) -> None:
    """Compose editor shows error for invalid YAML without writing to disk."""
    web, _port = start_web_server

    instance = f"pytest-web-{uuid.uuid4().hex[:8]}"
    compose_file = Path(web_test_env["COMPOSE_FILE"])
    write_single_service_compose(
        compose_file,
        instance=instance,
        image="ark-server:latest",
        admin_password="AdminPass123",
        server_password="",
        entrypoint='["sh", "-c", "echo ready && sleep 30"]',
    )

    original = compose_file.read_text()

    resp = web.post(
        f"/instances/{instance}/compose",
        data={"content": "this: is: not: valid: [yaml"},
    )
    assert resp.status_code == 200
    assert "Invalid compose YAML" in resp.text

    # File must not have been modified
    assert compose_file.read_text() == original


def test_static_files_serve(
    start_web_server: tuple[WebClient, int],
) -> None:
    """Static CSS and JS are served successfully."""
    web, _port = start_web_server

    resp = web.get("/static/pico.min.css")
    assert resp.status_code == 200
    assert len(resp.content) > 1000

    resp = web.get("/static/htmx.min.js")
    assert resp.status_code == 200
    assert len(resp.content) > 1000
