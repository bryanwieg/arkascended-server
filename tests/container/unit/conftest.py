from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest


def load_start_module():
    module_path = Path(__file__).resolve().parents[3] / "container/scripts/runtime/ark-server-start.py"
    spec = importlib.util.spec_from_file_location("ark_server_start", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Unable to load module from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture
def start_mod():
    return load_start_module()
