# Edit Server INI Files

`edit ini` opens the file in an editor inside the container (the running one,
or a temporary one when stopped), so volume ownership stays correct. Don't
edit the files from the host while the server runs.

```sh
ark-cli show ini --instance my-server --file gameusersettings   # print
ark-cli edit ini --instance my-server --file gameusersettings   # GameUserSettings.ini
ark-cli edit ini --instance my-server --file game               # Game.ini
```

- Editor: `nano`, the only one in the image. `--editor` runs another
  command from the image instead.
- `GameUserSettings.ini` must exist: the first start installs ARK's full
  default. Before that, `edit ini` refuses rather than create a partial file
  that would block the defaults.
- `Game.ini` is created on first edit; ARK never creates it.
- Changes apply on the next start.

## Running server

`edit ini` refuses while the instance runs, because the server can overwrite
the file. Override with `--force`; add `--restart` to restart after the
editor exits:

```sh
ark-cli edit ini --instance my-server --file gameusersettings --force --restart
```

Without `--restart`, run `ark-cli restart instance --instance my-server`.

## Keys managed from the environment

Every start rewrites these `[ServerSettings]` keys in
`GameUserSettings.ini`: `RCONEnabled`, `RCONPort`, `ServerAdminPassword`,
`ServerPassword`. Change the passwords with `ark-cli reconfigure instance`
(`--admin-password`, `--server-password`).

## File location

In the instance's `<name>-saved` volume:

```
ShooterGame/Saved/Config/WindowsServer/Game.ini
ShooterGame/Saved/Config/WindowsServer/GameUserSettings.ini
```

Host path: `ark-cli show volume-paths --instance my-server`.
