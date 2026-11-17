# Add a New CLI Command

How to add a new `ark-cli` verb/object command to the host CLI.

## Overview

All commands follow the `<verb> <object>` pattern (e.g. `create instance`,
`check reachability`). The routing table is a flat slice in
`app/ark-cli/internal/cmd/registry_entries.go`. Adding a command is three steps:
register the route, write the handler, write the tests.

## Step 1: Register the route

Add an entry to `CommandRegistry` in `app/ark-cli/internal/cmd/registry_entries.go`:

```go
{
    Verb:   "myverb",
    Object: "myobject",
    Route:  "myverb_myobject",
    Handler: mypkg.RunMyVerbMyObject,
    TopLevelHelp: []string{
        "myverb myobject <name>            One-line description for ark-cli --help",
    },
},
```

- `Route` is a unique string key; convention is `verb_object` with underscores (or bare verb when a verb has only one primary command).
- `TopLevelHelp` lines appear in `ark-cli --help` output sorted alphabetically
  by verb then object. Set to `nil` for hidden/compat aliases.

## Step 2: Write the handler

Add a function in a new or existing package under `app/ark-cli/internal/`:

```go
func RunMyVerbMyObject(d *core.AppDeps, args []string) error {
    fs := core.NewCommandFlagSet(
        "myverb myobject",
        []string{"myverb myobject --instance <instance> [options]"},
        "One-line description.",
    )
    instanceFlag := fs.String("instance", "", "Instance name.")
    if err := core.ParseNoPositionalArgs(fs, args); err != nil {
        return err
    }
    if *instanceFlag == "" {
        return errors.New("Error: --instance is required.")
    }
    name := *instanceFlag // use d.* seams for all side effects
    result, err := d.Runtime.RunCommand(
        []string{"docker", "inspect", name},
        ark.CommandOptions{Capture: true},
    )
    if err != nil {
        return err
    }
    core.Infof("%s\n", result.Stdout)
    return nil
}
```

Rules:
- Pass targets as flags (`--instance`, `--name`, `--backup`, `--port`), never
  positionally, and parse with `core.ParseNoPositionalArgs`. Every command
  uses flags so invocations are self-documenting and consistent.
- Access Docker, filesystem, and external state only through `d.*` seams, not
  package-level functions. This makes the handler testable without Docker.
- Keep the handler to ~10–20 lines; extract helpers for logic used elsewhere.
- Validate inputs before touching compose services or the filesystem.

### Delegating to a domain service

Most real commands delegate to a domain service rather than calling
`d.Runtime.RunCommand` directly. The handler validates inputs, calls the service,
and formats output.

```go
func RunMyVerbMyObject(d *core.AppDeps, args []string) error {
    fs := core.NewCommandFlagSet(
        "myverb myobject",
        []string{"myverb myobject --instance <instance> [options]"},
        "One-line description.",
    )
    instanceFlag := fs.String("instance", "", "Instance name.")
    composeFileFlag := fs.String("compose-file", "", "Path to compose file")
    if err := core.ParseNoPositionalArgs(fs, args); err != nil {
        return err
    }
    if *instanceFlag == "" {
        return errors.New("Error: --instance is required.")
    }
    name := *instanceFlag

    composeFile, project, err := compose.ResolveRequiredServiceConfig(
        d, *composeFileFlag, name, compose.ResolveRequiredComposeConfig,
    )
    if err != nil {
        return err
    }

    result, err := d.MyService.DoSomething(d.Context(), composeFile, name)
    if err != nil {
        return err
    }
    core.Infof("%s\n", result)
    return nil
}
```

The domain service (`d.MyService`) is injected via `AppDeps`. Domain services live
in `app/pkg/ark/` packages.

### Editing compose files

Commands that modify `docker-compose.yaml` must use `LoadProject` → mutate →
`SaveProject`. Never read or write compose files directly.

Use `compose.ResolveRequiredComposeConfig(d, *composeFileFlag)` when you only
need to load the compose project (no service name validation). Use
`compose.ResolveRequiredServiceConfig(d, *composeFileFlag, name, compose.ResolveRequiredComposeConfig)`
when the command targets a specific service — it loads the compose project and
validates that the named service exists.

```go
composeFile, project, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
if err != nil {
    return err
}

// Mutate the project in memory
if err := composeutil.MutateService(project, instance, func(svc *types.ServiceConfig) {
    composeutil.SetEnv(svc.Environment, "SESSION_NAME", *session)
}); err != nil {
    return err
}

if err := d.ComposeService.SaveProject(d.Context(), composeFile, project); err != nil {
    return err
}
```

For lower-level AST editing (inserting services, removing volumes), use the
package-level functions (`runtime.InsertService`, `runtime.RemoveServiceBlock`,
`runtime.EnsureVolumeEntry`) in `app/pkg/ark/runtime/compose_edit.go`.

### Adding `--json` output

Commands that display structured data should support a `--json` boolean flag.
Define a JSON struct in `app/ark-cli/internal/core/json_output.go` and branch
on the flag.

```go
// In the handler:
jsonFlag := fs.Bool("json", false, "Output as JSON.")

// After computing results:
if *jsonFlag {
    return core.PrintJSONOutput(core.MyResultJSON{
        Name:   name,
        Status: status,
    })
}
// Otherwise print text table
core.PrintTable(rows)
```

The JSON struct must have `json` tags. Follow the naming pattern `XxxJSON` for
the struct name.

## Step 3: Write tests

### Contract test (no Docker required)

```go
func TestMyVerbMyObjectDoesTheThing(t *testing.T) {
    var gotCmd []string
    testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, func(d *core.AppDeps) {
        d.Runtime.RunCommand = func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
            gotCmd = cmd
            return ark.ExecResult{ExitCode: 0, Stdout: "ok"}, nil
        }
    }, func() {
        err := mypkg.RunMyVerbMyObject(core.CurrentAppDeps(), []string{"asa1"})
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
    })
    if len(gotCmd) == 0 || gotCmd[1] != "inspect" {
        t.Fatalf("unexpected command: %v", gotCmd)
    }
}
```

### Integration test (Docker required)

Integration tests live in `tests/app/ark_cli/integration/docker/fast/` (fast checks, no ARK server needed) and `tests/app/ark_cli/integration/docker/slow/` (full ARK server lifecycle). Podman-runtime equivalents re-export the same tests from `tests/app/ark_cli/integration/podman/fast/` and `tests/app/ark_cli/integration/podman/slow/`. Two examples:

- **Fast** (`fast/`) — compose manipulation, fake servers, no running ARK server.
  Use these when you can fully verify behavior with mocked APIs or lightweight
  fixtures. Mark with `@pytest.mark.container`.
- **Slow** (`slow/`) — real Docker builds, real API calls, real container
  lifecycle. **Every feature that touches Docker images, external APIs, or
  container behavior must have slow tests** that prove it works against
  real infrastructure. Mark with `@pytest.mark.container`.

Example: the `update instance` command has fast tests in
`tests/app/ark_cli/integration/docker/fast/test_update_local_check.py` that use a fake
Steam API server, and slow tests in
`tests/app/ark_cli/integration/docker/slow/test_update_local_check.py` that call the real
Steam API and run a real `docker buildx build`.

For Go-level integration tests that need a real container, use the `integration`
build tag in `app/pkg/ark/conformance/`. CLI-level integration tests are Python
tests in `tests/app/ark_cli/integration/`.

## Step 4: Update documentation

- Add the new command to `docs/user/reference/cli-commands.md` with example
  invocations.
- If the command affects server lifecycle or is user-facing, add a how-to
  guide under `docs/user/how-to/`.

## File size check

After adding your files, verify they stay within the ~300–350 line limit.
Split by responsibility if they grow beyond that.

## Compose editing

Commands that modify compose files must use the `ComposeService.LoadProject` /
`ComposeService.SaveProject` round-trip (see `app/pkg/ark/compose_service.go`).
Do not load or save compose files directly, and do not hand-roll YAML string
manipulation. See the "Editing compose files" subsection above for a code example.