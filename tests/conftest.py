from __future__ import annotations

import inspect
import json
import os
import re
import shutil
import subprocess
import tempfile
import time
from collections.abc import Callable, Generator
from pathlib import Path

import pytest
import yaml
from helpers.artifacts import (
    artifacts_retention,
    ensure_cli_binary,
    prune_test_artifacts,
    test_node_dir_name,
    tests_root,
    wipe_test_artifacts,
    write_pytest_report_artifact,
)
from helpers.artifacts import (
    repo_root as _repo_root,
)
from helpers.framework_logger import configure as _configure_framework_logger
from helpers.framework_logger import log_framework as _log_framework

# pytest-timeout is used globally (see pytest.ini timeout setting).  Tests in a
# slow/ directory involve a running ARK server container, which can legitimately
# take up to 30 minutes.  We extend the default timeout for these tests to
# 1800 seconds centrally here instead of spreading @pytest.mark.timeout() on
# every slow test file.
_SLOW_TEST_TIMEOUT = 1800


_INTEGRATION_DIRS = {"integration"}
_FRAMEWORK_WARNINGS: list[str] = []

# Test ordering rules — loaded from .test-order.yaml
_ORDER_FILE = Path(__file__).parent / ".test-order.yaml"
_TEST_ORDER: dict[str, list[str]] = {}
if _ORDER_FILE.is_file():
    loaded = yaml.safe_load(_ORDER_FILE.read_text())
    if isinstance(loaded, dict):
        _TEST_ORDER = loaded

# Run-state directory for tracking test outcomes across invocations; it sits in the artifacts dir
# (ARK_TEST_ARTIFACTS_DIR, default .tests/) and the retention wipe keeps it.
_RUN_STATE_DIR = tests_root(_repo_root()) / ".run-state"

# Wipe state on explicit request (ARK_TEST_WIPE_STATE=1)
if os.environ.get("ARK_TEST_WIPE_STATE", "").lower() in ("1", "true", "yes"):
    if _RUN_STATE_DIR.is_dir():
        shutil.rmtree(_RUN_STATE_DIR)
    _log_framework("framework", "run-state wiped via ARK_TEST_WIPE_STATE")

# In-memory failure tracker for within-session dependency skipping.
# Records test failures immediately so subsequent tests in the same
# session see them without waiting for the state file write to persist.
_FAILED_TESTS: dict[str, set[str]] = {}  # group -> set of failed test names


def _test_group_for(item: pytest.Item) -> str | None:
    """Return the order group key for an item, or None if not ordered."""
    parts = Path(item.fspath).relative_to(Path(__file__).parent).parts
    for candidate_depth in range(len(parts), 0, -1):
        candidate = str(Path(*parts[:candidate_depth]))
        if candidate in _TEST_ORDER:
            return candidate
    return None


def _state_path(group: str) -> Path:
    """Return the path to the run-state JSON file for a group."""
    safe_name = group.replace("/", "_")
    path = _RUN_STATE_DIR / f"{safe_name}.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


@pytest.hookimpl(tryfirst=True)
def pytest_configure(config: pytest.Config) -> None:
    from helpers.compose_cleanup import install_signal_handlers

    install_signal_handlers()
    config._framework_warnings = []


def pytest_terminal_summary(terminalreporter: pytest.TerminalReporter, config: pytest.Config) -> None:
    warnings = getattr(config, "_framework_warnings", [])
    if warnings:
        terminalreporter.section("Framework compliance warnings", sep="-")
        for w in warnings:
            terminalreporter.line(f"  ⚠  {w}")


def _is_integration_test(item: pytest.Item) -> bool:
    parts = Path(item.fspath).parts
    return any(p in _INTEGRATION_DIRS for p in parts) and Path(item.fspath).name.startswith("test_")


def pytest_collection_modifyitems(config: pytest.Config, items: list[pytest.Item]) -> None:  # noqa: PLR0912, PLR0915
    # Artifact retention: wipe for suite runs, prune for individual runs
    root = _repo_root()
    retention = artifacts_retention()
    wipe_threshold = int(os.environ.get("ARK_TEST_ARTIFACTS_WIPE_THRESHOLD", "3"))
    if len(items) > wipe_threshold:
        wipe_test_artifacts(root, preserve_recent=retention)
    elif retention > 0:
        prune_test_artifacts(root, retention=retention)

    # Lite mode: tests that check the real ARK server can't run on the fake one.
    from helpers.lite_image import lite_mode

    if lite_mode():
        skip_real = pytest.mark.skip(reason="needs the real ARK server (ARK_TEST_LITE is set)")
        for item in items:
            if item.get_closest_marker("real_server"):
                item.add_marker(skip_real)

    # Apply extended timeout to all slow tests (involving a running ARK server).
    # This centralizes timeout management instead of requiring @pytest.mark.timeout()
    # on every slow test file.
    for item in items:
        if "slow" in Path(item.fspath).parts:
            item.add_marker(pytest.mark.timeout(_SLOW_TEST_TIMEOUT))

    # Attach a mutable list for failure-diagnostics callbacks to each item.
    # Fixtures register into this list; on failure the hook runs them.
    for item in items:
        item._ark_diagnostics_callbacks = []

    checked_modules: dict[str, str] = {}
    for item in items:
        if not _is_integration_test(item):
            continue
        fspath = str(item.fspath)
        fname = Path(fspath).name

        # Read source once per file, cache for subsequent checks
        if fspath not in checked_modules:
            try:
                source = inspect.getsource(item.module)
            except OSError, TypeError:
                source = ""
            checked_modules[fspath] = source
        source = checked_modules[fspath]

        # Rule 1: No raw subprocess in integration test files (native/ tests are exempt — they use run_cmd_native)
        is_native = "native" in Path(fspath).parts
        if ("subprocess.run" in source or "subprocess.Popen" in source) and not is_native:
            pytest.fail(f"{fname}: uses raw subprocess — use run_cmd fixture instead")

        # Rule 2: Required container marker (native/ tests are exempt — no container runtime)
        markers = {m.name for m in item.iter_markers()}
        if "container" not in markers and not is_native:
            pytest.fail(f"{item.nodeid}: missing @pytest.mark.container marker")

        # Rule 3: CLI integration tests should use test_env fixture (native/ tests are exempt)
        fixture_names = getattr(item, "fixturenames", [])
        if "ark_cli" in Path(fspath).parts and not is_native:
            has_tmp_path = "tmp_path" in fixture_names
            has_test_env = "test_env" in fixture_names
            if has_tmp_path and not has_test_env:
                pytest.fail(
                    f"{item.nodeid}: CLI integration test uses tmp_path without "
                    f"test_env — use test_env fixture for consistent "
                    f"environment setup (ARK_BACKUP_DIR, COMPOSE_FILE, artifact paths). "
                    f"If this is intentional, add a comment explaining why.",
                )

        # Rule 4: Direct os.environ access in integration test files
        # Exempt: podman compatibility tests do their own runtime detection
        # Exempt: native/ tests manage their own environment without test_env
        if "os.environ" in source and "podman_compat" not in fspath and not is_native:
            pytest.fail(
                f"{fname}: uses os.environ directly — use test_env.get() or "
                f"dict(test_env) instead for consistent environment setup. "
                f"If this is a justified exception (e.g. Podman tests), add a "
                f"comment explaining why.",
            )

        # Rule 5: In-place mutation of test_env dict
        # Match test_env["KEY"] = or test_env['KEY'] = but not reads
        if re.search(r"test_env\[.+\]\s*=", source):
            pytest.fail(
                f"{fname}: mutates test_env dict in-place — "
                f"use env = dict(test_env); env[KEY] = VALUE instead to "
                f"avoid state leakage between tests.",
            )

        # Rule 6: Hardcoded "docker" string in command lists in test files
        # Detect single-line patterns like ["docker", "compose", ...]
        docker_cmd_pattern = re.compile(r'\["docker"\s*,\s*"(?:compose|inspect|exec|tag|image)"')
        matches = docker_cmd_pattern.findall(source)
        if matches:
            pytest.fail(
                f"{fname}: hardcodes '{matches[0]}' — "
                f"use compose_client SDK fixture instead ({len(matches)} occurrences found)."
            )

        # Rule 6b: Multi-line "docker" command lists
        # Catches patterns like:
        #   [
        #       "docker",
        #       "compose",
        #       ...
        #   ]
        docker_lines = [line for line in source.splitlines() if re.match(r'^\s*"docker",\s*$', line)]
        if docker_lines:
            pytest.fail(
                f"{fname}: uses multi-line ['docker', ...] ({len(docker_lines)} occurrences) "
                f"— use compose_client SDK fixture instead."
            )

        # Rule 7: Hardcoded "podman" strings in command lists
        # Exempt: podman_compat.py does its own runtime detection
        if "podman_compat" not in fspath:
            podman_cmd_pattern = re.compile(r'\["podman"\s*,\s*"(?:compose|inspect|exec|tag|image)"')
            if podman_cmd_pattern.findall(source):
                pytest.fail(f"{fname}: hardcodes ['podman', ...] — use compose_client SDK fixture instead.")
            if re.search(r'^\s*"podman",\s*$', source, re.MULTILINE):
                pytest.fail(f"{fname}: uses multi-line ['podman', ...] — use compose_client SDK fixture instead.")

        # Rule 8: Container tests must not import cli_path or ark-cli
        # Container tests exercise scripts directly, never through the CLI.
        if "container" in Path(fspath).parts:  # noqa: SIM102
            if "cli_path" in source and "podman_compat" not in fspath:
                pytest.fail(
                    f"{fname}: container test imports cli_path — "
                    f"container tests must exercise container scripts directly, "
                    f"not through ark-cli. Use docker primitives instead."
                )

        # Rule 9: Slow tests must include slow_test_protection fixture
        needs_lock = "slow" in Path(fspath).parts
        has_lock = "slow_test_protection" in fixture_names
        if needs_lock and not has_lock:
            pytest.fail(
                f"{item.nodeid}: slow test missing slow_test_protection fixture — "
                f"add it to prevent OOM from concurrent slow tests."
            )

        # Rule 9b: No tempfile in integration tests. Its dirs live in /tmp, which the Docker
        # daemon may not see, so a bind mount from them comes up empty and root-owned.
        if re.search(r"\btempfile\b|TemporaryDirectory|mkdtemp|gettempdir", source):
            pytest.fail(
                f"{fname}: uses tempfile — use the test_artifact_dir or tmp_path fixture instead; "
                f"both live under .tests/, which containers can bind-mount."
            )

        # Rule 10: File complexity checks (warnings, not failures)
        # Large or complex test files often contain inline helper logic
        # that should live in tests/helpers/ for reuse across the suite.
        file_warnings: list[str] = []

        # 10a: File is large
        char_count = len(source)
        if char_count > 2500:
            file_warnings.append(f"{char_count} characters — consider extracting shared logic into tests/helpers/")

        # 10b: Many test functions per file
        fn_count = source.count("def ")
        if fn_count > 6:
            file_warnings.append(f"{fn_count} functions — consider splitting into smaller test files")

        # 10c: Inline wait_for loops that should use the helper
        if re.search(r"time\.monotonic\(\)", source):
            file_warnings.append("inline timeout loop — use wait_for helper instead")

        # Rule 11: Session-scoped fixtures in conftest files (warnings)
        # Session-scoped fixtures can cause state leakage. Only known-safe
        # fixtures (immutable values or idempotent setup) are allowed without
        # a comment explaining why the scope is safe.
        if fname == "conftest.py":
            _SESSION_SCOPE_SAFE = {  # noqa: N806
                "repo_root",
                "cli_path",
                "test_run_dir",
                "_ensure_container_image",
                "_ensure_podman_image",
                "_set_docker_env",
                "web_binary",
            }
            for match in re.finditer(
                r'@pytest\.fixture\(scope="session.*?\)\s*\n\s*def\s+(\w+)',
                source,
            ):
                fixture_name = match.group(1)
                if fixture_name not in _SESSION_SCOPE_SAFE:
                    file_warnings.append(
                        f"session-scoped fixture '{fixture_name}' — "
                        f"prefer function scope unless the fixture returns "
                        f"an immutable value or performs idempotent setup"
                    )

        if file_warnings:
            config._framework_warnings.append(f"{fname}:" + "".join(f"\n    - {w}" for w in file_warnings))

    # Reorder items to match .test-order.yaml
    ordered: list[pytest.Item] = []
    unordered: list[pytest.Item] = []
    for item in items:
        group = _test_group_for(item)
        if group is None:
            unordered.append(item)
        else:
            ordered.append(item)

    if ordered:
        order_map: dict[str, list[pytest.Item]] = {}
        for item in ordered:
            group = _test_group_for(item)
            if group not in order_map:
                order_map[group] = []
            order_map[group].append(item)

        reordered: list[pytest.Item] = []
        for group, order_list in _TEST_ORDER.items():
            if group not in order_map:
                continue
            items_for_group = order_map[group]
            for test_name in order_list:
                for item in items_for_group[:]:
                    if Path(item.fspath).stem == test_name:
                        reordered.append(item)
                        items_for_group.remove(item)
            # Append any remaining group items not in the order list
            reordered.extend(items_for_group)

        items[:] = reordered + unordered


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_makereport(item: pytest.Item, call: pytest.CallInfo[object]) -> Generator[None]:  # noqa: PLR0912
    outcome = yield
    report = outcome.get_result()
    setattr(item, f"rep_{report.when}", report)

    # Record test outcome in run-state for dependency tracking
    if report.when == "call":
        group = _test_group_for(item)
        if group is not None:
            test_name = Path(item.fspath).stem

            # Record in-memory (instant — available for next test in session)
            if not report.passed and group in _TEST_ORDER:
                _FAILED_TESTS.setdefault(group, set()).add(test_name)

            # Record on-disk (persists across separate pytest invocations)
            state_path = _state_path(group)
            state: dict[str, str] = {}
            if state_path.is_file():
                state = json.loads(state_path.read_text())
            state[test_name] = "passed" if report.passed else "failed"
            # Atomic write via tempfile + rename
            fd, tmp_path = tempfile.mkstemp(dir=str(state_path.parent), suffix=".tmp")
            try:
                with os.fdopen(fd, "w") as f:
                    json.dump(state, f)
                os.rename(tmp_path, str(state_path))  # noqa: PTH104
            except Exception:  # noqa: BLE001
                if os.path.exists(tmp_path):  # noqa: PTH110
                    os.unlink(tmp_path)  # noqa: PTH108

        # Capture diagnostics immediately on test call failure
        if not report.passed and report.when == "call":
            artifact_dir = getattr(item, "_ark_test_artifact_dir", None)
            if artifact_dir:
                try:
                    # Access test_env from the fixture request to get compose_file
                    request = getattr(item, "_request", None)
                    test_env = (
                        request.getfixturevalue("test_env") if request and "test_env" in request.fixturenames else None
                    )
                except Exception:  # noqa: BLE001
                    test_env = None
                if test_env:
                    compose_file = Path(test_env.get("COMPOSE_FILE", ""))
                    if compose_file.is_file():
                        from helpers.compose_client import get_docker_client
                        from helpers.diagnostics import capture_diagnostics

                        dc = get_docker_client(compose_file=compose_file)
                        failure_log = artifact_dir / "failure.log"
                        capture_diagnostics(dc, compose_file, failure_log)
                        _log_framework("framework", "captured failure diagnostics to %s", failure_log)

    # Write the pytest report artifact after all phases complete, so that
    # setup, call, and teardown reports are all available.
    if report.when == "teardown":
        artifact_dir = getattr(item, "_ark_test_artifact_dir", None)
        if artifact_dir:
            write_pytest_report_artifact(item, artifact_dir)


def pytest_runtest_setup(item: pytest.Item) -> None:
    """Skip test if a dependency in its order group has failed."""
    group = _test_group_for(item)
    if group is not None and group in _TEST_ORDER:
        order = _TEST_ORDER[group]
        test_name = Path(item.fspath).stem
        if test_name in order:
            test_index = order.index(test_name)
            if test_index > 0:
                for dep_name in order[:test_index]:
                    # Check in-memory first (instant — within same session)
                    if dep_name in _FAILED_TESTS.get(group, set()):
                        pytest.skip(f"Skipping: dependency {dep_name} failed")
                    # Fall back to on-disk state (cross-session)
                    state_path = _state_path(group)
                    state: dict[str, str] = {}
                    if state_path.is_file():
                        state = json.loads(state_path.read_text())
                    if state.get(dep_name) == "failed":
                        pytest.skip(f"Skipping: dependency {dep_name} failed")


@pytest.fixture
def compose_client(test_env: dict[str, str]) -> DockerClient:
    """Returns a python-on-whales DockerClient wired to the test's compose file.

    The project name is derived from the compose file path by both
    ``python-on-whales`` and ``docker compose -f <file>``, keeping
    them in sync.
    """
    from helpers.compose_client import get_docker_client

    compose_file = Path(test_env["COMPOSE_FILE"])
    return get_docker_client(compose_file=compose_file)


@pytest.fixture
def run_cmd() -> Callable[..., subprocess.CompletedProcess[str]]:
    """Root-level run_cmd fixture for standalone baseline tests."""
    from helpers.diagnostics import run_with_compose_patch

    return run_with_compose_patch


@pytest.fixture
def test_env(
    request: pytest.FixtureRequest,
    repo_root: Path,
    test_artifact_dir: Path,
) -> Generator[dict[str, str]]:
    """Minimal test_env for tests that need container infrastructure.

    Full container-level setup is in container/integration/conftest.py.
    This root-level fixture provides an env dict with the basic config
    for tests that perform lightweight container operations.
    """
    from helpers.compose_client import get_docker_client
    from helpers.infra import cleanup_test_env, prepare_test_env
    from helpers.lite_image import image_for_test

    dc = get_docker_client()
    env, compose_file = prepare_test_env(
        dc,
        compose_dir=test_artifact_dir,
        backup_dir=repo_root / "backups",
        compose_prefix="pytest-compose-",
        image=image_for_test(request),
    )
    yield env
    cleanup_test_env(dc, env, compose_file)


@pytest.fixture(scope="session")
def repo_root() -> Path:
    return _repo_root()


@pytest.fixture(scope="session")
def cli_path(repo_root: Path) -> Path:
    return ensure_cli_binary(repo_root)


@pytest.fixture(scope="session")
def test_run_dir(repo_root: Path, request: pytest.FixtureRequest) -> Path:
    batch_label = os.environ.get("ARK_TEST_BATCH_LABEL")
    override = os.environ.get("ARK_TEST_RUN_DIR")
    if override and batch_label:
        path = Path(override)
        path.mkdir(parents=True, exist_ok=True)
        from helpers.artifacts import sanitize_path_component

        batch_dir = path / sanitize_path_component(batch_label)
        batch_dir.mkdir(parents=True, exist_ok=True)
        return batch_dir
    if override:
        path = Path(override)
        path.mkdir(parents=True, exist_ok=True)
        return path
    timestamp = time.strftime("%y%m%d-%H%M%S", time.localtime())
    root = tests_root(repo_root)
    items = getattr(request.session, "items", [])
    suffix_name = "suiterun" if len(items) != 1 else "singlerun"
    run_dir = root / f"{timestamp}-{suffix_name}"
    suffix = 1
    while run_dir.exists():
        suffix += 1
        run_dir = root / f"{timestamp}-{suffix_name}-{suffix}"
    run_dir.mkdir(parents=True, exist_ok=True)
    return run_dir


@pytest.fixture
def test_artifact_dir(test_run_dir: Path, request: pytest.FixtureRequest) -> Path:
    node_dir = test_run_dir / test_node_dir_name(request.node.nodeid)
    node_dir.mkdir(parents=True, exist_ok=True)
    return node_dir


@pytest.fixture
def tmp_path(test_artifact_dir: Path) -> Path:
    """Per-test scratch dir under .tests/, replacing pytest's /tmp-based tmp_path.

    The Docker daemon may not see the caller's /tmp (a remote or rootless daemon), so a
    bind mount from it comes up empty. .tests/ is in the repo, which containers can mount,
    and is pruned by the artifact retention.
    """
    path = test_artifact_dir / "tmp"
    path.mkdir(exist_ok=True)
    return path


@pytest.fixture(autouse=True)
def configure_test_artifacts(
    request: pytest.FixtureRequest,
    test_artifact_dir: Path,
) -> Generator[None]:
    # Configure framework debug logger for this test
    framework_log_path = test_artifact_dir / "framework.log"
    _configure_framework_logger(str(framework_log_path))
    is_slow = "slow" in Path(request.node.fspath).parts
    _log_framework("framework", "test %s started (slow=%s)", request.node.nodeid, is_slow)

    for fixture_name in ("test_env",):
        if fixture_name not in request.fixturenames:
            continue
        env = request.getfixturevalue(fixture_name)
        env["ARK_TEST_ARTIFACT_DIR"] = str(test_artifact_dir)
        env["ARK_TEST_CLI_LOG"] = str(test_artifact_dir / "cli.log")
        env["ARK_TEST_DOCKER_LOG"] = str(test_artifact_dir / "docker.log")
        env["ARK_TEST_COMMANDS_LOG"] = str(test_artifact_dir / "commands.log")
        env["ARK_TEST_FRAMEWORK_LOG"] = str(framework_log_path)

    # Store test_artifact_dir on the item for failure-diagnostics and snapshot capture
    request.node._ark_test_artifact_dir = test_artifact_dir

    yield

    _log_framework("framework", "test %s finished", request.node.nodeid)


from typing import TYPE_CHECKING  # noqa: E402

if TYPE_CHECKING:
    from python_on_whales import DockerClient

    _close_framework_logger()  # noqa: F821
