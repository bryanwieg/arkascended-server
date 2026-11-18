# Add a New CLI Command

Commands are `<verb> <object>` (`create instance`, `check reachability`).
Routing is a flat slice, `CommandRegistry`, in
`app/ark-cli/internal/cmd/registry_entries.go`.

## Step 1: Register the route

Add an entry to `CommandRegistry`:

```go
{Verb: "myverb", Object: "myobject", Route: "myverb_myobject", Handler: mypkg.RunMyVerbMyObject, TopLevelHelp: []string{
    "myverb myobject --instance <inst>          One-line description for ark-cli --help",
}},
```

- `Handler` has the signature `func(*core.AppDeps, []string) error`.
- `Route`: unique key. Use `verb_object`. Some older entries use a bare
  verb (`create`, `start`, `restore`).
- `TopLevelHelp`: lines for `ark-cli --help`, sorted by verb, then object.
  Use `nil` for hidden or compatibility aliases (see `show vol-paths`).

The registry tests in `app/ark-cli/internal/cmd/registry_test.go` check
that every route has a handler and that every verb has a parse spec.

## Step 2: Write the handler

Put it in a new or existing package under `app/ark-cli/internal/`. The
example below sets an environment variable on one compose service:

```go
func RunMyVerbMyObject(d *core.AppDeps, args []string) error {
    fs := core.NewCommandFlagSet(
        "myverb myobject",
        []string{"myverb myobject --instance <instance> --session <name>"},
        "One-line description.",
    )
    instanceFlag := fs.String("instance", "", "Instance name.")
    sessionFlag := fs.String("session", "", "Session name.")
    composeFileFlag := fs.String("compose-file", "", "Path to compose file.")
    if err := core.ParseNoPositionalArgs(fs, args); err != nil {
        return err
    }
    if *instanceFlag == "" {
        return errors.New("Error: --instance is required.")
    }
    instance := *instanceFlag

    // Loads via ComposeService and checks that the service exists.
    composeFile, project, err := compose.ResolveRequiredServiceConfig(
        d, *composeFileFlag, instance, compose.ResolveRequiredComposeConfig,
    )
    if err != nil {
        return err
    }
    if err := composeutil.MutateService(project.Project, instance, func(svc *types.ServiceConfig) {
        composeutil.SetEnv(svc.Environment, "SESSION_NAME", *sessionFlag)
    }); err != nil {
        return err
    }
    if err := d.ComposeService.SaveProject(d.Context(), composeFile, project); err != nil {
        return err
    }
    core.Infof("Updated %s in %s\n", instance, composeFile)
    return nil
}
```

Rules:
- Targets are flags (`--instance`, `--name`, `--backup`), never positional
  arguments. Parse them with `core.ParseNoPositionalArgs`.
- Reach Docker, the filesystem and other external state only through `d.*`
  seams: `d.ComposeService`, `d.Engine`, the domain services (`d.Lifecycle`,
  `d.BackupService`, `d.UpdateService`, `d.ScheduleService`,
  `d.HealthChecker`) and `d.Runtime.RunCommand` for host commands. See
  `AppDeps` in `app/ark-cli/internal/core/deps.go`.
- Keep handlers short (about 10–20 lines): parse, validate, call a
  service, print. Put domain logic in a service under `app/pkg/ark/` and
  wire it into `AppDeps`.
- Validate inputs before you touch compose services or the filesystem.
- Print user-facing output with `core.Infof`, `core.Infoln` or `core.Warnf`
  ([logging architecture](../explanation/logging-architecture.md)).

### Compose files

- To edit an existing service, load the project, change it in memory, then
  save it with `ComposeService.SaveProject`, as in the example. Do not use
  direct file I/O or hand-written YAML string edits.
- `compose.ResolveRequiredComposeConfig(d, flag)` only loads the project.
  `compose.ResolveRequiredServiceConfig(...)` also checks that the named
  service exists. Both respect `--compose-file`, then `COMPOSE_FILE`.
- To add or remove a whole service block, use the text-level helpers in
  `app/pkg/ark/runtime/compose_edit.go` (`InsertService`,
  `RemoveServiceBlock`, `EnsureVolumeEntry`, `RemoveVolumeEntry`), as
  `create instance`, `remove instance` and `restore backup --new-instance`
  do.

### `--json` output

Commands that show structured data should take a `--json` bool flag. Add
an `XxxJSON` struct with `json` tags to
`app/ark-cli/internal/core/json_output.go`:

```go
jsonFlag := fs.Bool("json", false, "Output as JSON.")
// ...
if *jsonFlag {
    return core.PrintJSONOutput(core.MyResultJSON{Name: name, Status: status})
}
core.PrintTable(rows) // first row is the header
```

## Step 3: Write tests

### Go contract test

Use `testutil.WithPatchedAppDeps` to swap seams. No Docker is needed.

```go
func TestMyVerbMyObjectSetsSessionName(t *testing.T) {
    var saved *ark.ComposeProject
    testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, func(d *core.AppDeps) {
        d.ComposeService = &arktest.MockCompose{
            LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
                return &ark.ComposeProject{Project: &types.Project{Services: types.Services{
                    "asa1": {Name: "asa1", Environment: types.MappingWithEquals{}},
                }}}, nil
            },
            SaveProjectFn: func(_ context.Context, _ string, p *ark.ComposeProject) error {
                saved = p
                return nil
            },
        }
    }, func() {
        err := RunMyVerbMyObject(core.CurrentAppDeps(), []string{"--instance", "asa1", "--session", "Test"})
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
    })
    if saved == nil || *saved.Project.Services["asa1"].Environment["SESSION_NAME"] != "Test" {
        t.Fatalf("SESSION_NAME not saved: %+v", saved)
    }
}
```

`TestDepsConfig` presets (`FullConfig`, `ComposeOnlyConfig`, and others)
are in `app/ark-cli/internal/testutil/deps.go`.

### Python integration tests

These are CLI-level tests in `tests/app/ark_cli/integration/`. For
placement and markers, see
[test architecture](../reference/test-architecture.md#test-layers):

| Directory | Use for |
|---|---|
| `native/` | No container runtime needed (argument parsing, help output, port checks) |
| `docker/fast/` | Compose manipulation, fake API servers, no running ARK server |
| `docker/slow/` | Real images, real external APIs, full container lifecycle |
| `podman/fast/`, `podman/slow/` | Re-export the Docker tests under Podman (`from ...docker.fast.test_x import *`) |

A feature that touches Docker images, external APIs or container behavior
needs slow tests against real infrastructure. For example, `update
instance` has `docker/fast/test_update_local_check.py`, which uses a fake
Steam API server. It also has `docker/slow/test_update_local_check.py`,
which calls the real Steam API and builds a real image.

Go integration tests that need a real container use the `integration`
build tag (backend conformance: `app/pkg/ark/conformance/`).

## Step 4: Update documentation

- Add the command and example invocations to
  [cli-commands.md](../../user/reference/cli-commands.md).
- If the command is user-facing or affects server lifecycle, add a how-to
  under [docs/user/how-to/](../../user/how-to/).
