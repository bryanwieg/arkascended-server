from __future__ import annotations


def test_create_instance_custom_compose_file(run_cli, native_workdir):
    """create instance with --compose-file writes to a custom path.

    Scenario:
        create instance is invoked with --compose-file pointing to a
        non-default path. The custom file is checked for existence and
        content.

    Expected:
        - The command exits zero.
        - The custom compose file exists and contains the instance service name.
    """
    custom = native_workdir / "custom-compose.yaml"
    native_workdir.mkdir(parents=True, exist_ok=True)
    result = run_cli(
        [
            "create",
            "instance",
            "--name",
            "custom1",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
            "--compose-file",
            str(custom),
        ],
        env_extra={"COMPOSE_FILE": str(custom)},
        check=False,
    )
    assert result.returncode == 0
    assert custom.exists()
    assert "custom1:" in custom.read_text()


def test_create_instance_default_values(run_cli, native_workdir):
    """create instance applies default env var values (MAP_NAME, TZ).

    Scenario:
        An instance is created without specifying map or timezone overrides.
        The generated compose file is checked for default values.

    Expected:
        - MAP_NAME defaults to 'TheIsland_WP'.
        - TZ defaults to 'America/New_York'.
    """
    run_cli(
        [
            "create",
            "instance",
            "--name",
            "def1",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ],
        check=True,
    )
    compose = (native_workdir / "docker-compose.yaml").read_text()
    assert "MAP_NAME=TheIsland_WP" in compose
    assert "TZ=America/New_York" in compose
