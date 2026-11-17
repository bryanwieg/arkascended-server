"""Container image build tests for Docker and Podman runtimes.

Verifies the two-stage build process produces a valid image under each runtime,
building it first if missing.
"""

from __future__ import annotations

import pytest
from helpers.build import build_server_image, image_exists
from helpers.compose_client import get_docker_client

pytestmark = [pytest.mark.container]

_DEFAULT_INI = "/home/ark-user/config-defaults/GameUserSettings.ini"


def test_build_container_image_docker(
    test_env: dict,
    slow_test_protection,
) -> None:
    """Build the container image using Docker if missing, and check its baked default GameUserSettings.ini.

    The default is the complete file ARK writes on a clean shutdown (it has [ServerSettings]),
    without the random SessionName ARK makes up during the build.
    """
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    if not image_exists(image):
        build_server_image(image)
    assert image_exists(image), f"Build completed but image '{image}' not found"
    default_ini = get_docker_client().run(image, [_DEFAULT_INI], entrypoint="cat", remove=True)
    assert "[ServerSettings]" in default_ini, f"Default INI is not ARK's complete file:\n{default_ini}"
    assert "SessionName=" not in default_ini, f"Default INI still has a SessionName:\n{default_ini}"


@pytest.mark.podman
def test_build_container_image_podman(
    test_env: dict,
    slow_test_protection,
) -> None:
    """Build the container image using Podman if missing."""
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    if not image_exists(image, runtime="podman"):
        build_server_image(image, runtime="podman")
    assert image_exists(image, runtime="podman"), f"Build completed but image '{image}' not found"
