from __future__ import annotations

import os
import subprocess
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.artifacts import ensure_cli_binary
from helpers.native_fixtures import SCHEDULE_COMPOSE_BASE


@pytest.fixture(scope="session")
def cli_path(repo_root: Path) -> Path:
    return ensure_cli_binary(repo_root)


@pytest.fixture(scope="session")
def repo_root() -> Path:
    from helpers.artifacts import repo_root as _repo_root

    return _repo_root()


@pytest.fixture
def run_cmd_native() -> Callable[..., subprocess.CompletedProcess[str]]:
    def _run(
        cmd: list[str],
        *,
        check: bool = True,
        capture_output: bool = False,
        timeout: int = 30,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            cmd,
            check=check,
            capture_output=capture_output,
            text=True,
            timeout=timeout,
        )

    return _run


@pytest.fixture
def native_workdir(tmp_path: Path) -> Path:
    return tmp_path / "native-workdir"


@pytest.fixture
def schedule_workdir(native_workdir: Path) -> Path:
    native_workdir.mkdir(parents=True, exist_ok=True)
    manage_dir = native_workdir / ".ark-manage"
    (manage_dir / "state" / "schedules").mkdir(parents=True, exist_ok=True)
    (manage_dir / "logs" / "schedules").mkdir(parents=True, exist_ok=True)
    (manage_dir / "locks").mkdir(parents=True, exist_ok=True)
    (manage_dir / "generated").mkdir(parents=True, exist_ok=True)
    (native_workdir / "docker-compose.yaml").write_text(SCHEDULE_COMPOSE_BASE)
    return native_workdir


@pytest.fixture
def backup_workdir(native_workdir: Path) -> Path:
    native_workdir.mkdir(parents=True, exist_ok=True)
    backups_dir = native_workdir / "backups"
    backups_dir.mkdir(parents=True, exist_ok=True)
    return backups_dir


@pytest.fixture
def sched_path(schedule_workdir: Path) -> Path:
    return schedule_workdir / "docker-compose.yaml"


@pytest.fixture
def run_cli(
    cli_path: Path,
    native_workdir: Path,
) -> Callable[..., subprocess.CompletedProcess[str]]:
    native_workdir.mkdir(parents=True, exist_ok=True)

    def _run(
        args: list[str],
        *,
        env_extra: dict[str, str] | None = None,
        check: bool = False,
        timeout: int = 30,
    ) -> subprocess.CompletedProcess[str]:
        full_cmd = [str(cli_path)] + args  # noqa: RUF005
        compose_file = str(native_workdir / "docker-compose.yaml")
        env = {
            "COMPOSE_FILE": compose_file,
            "ARK_BACKUP_DIR": str(native_workdir / "backups"),
        }
        if env_extra:
            env.update(env_extra)
        return subprocess.run(
            full_cmd,
            capture_output=True,
            text=True,
            cwd=str(native_workdir),
            env={**os.environ, **env},
            check=check,
            timeout=timeout,
        )

    return _run
