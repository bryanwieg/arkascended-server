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
- Which keys exist and their defaults:
  [Server INI settings](../reference/server-ini-settings.md). ARK writes
  only some of them; add the others yourself.

## Read or change one key

`get ini` shows a single key (every line for it, and whether ARK wrote it);
`set ini` changes it without an editor and reports the old value:

```sh
ark-cli get ini --instance my-server --file gus --section ServerSettings --key MaxPlayers
ark-cli set ini --instance my-server --file gus --section ServerSettings --key MaxPlayers --value 20
```

If the key appears more than once, `set ini` stops and lists the lines, marking
which ones ARK wrote (`get ini` shows the same list without changing anything). Some keys are lists (one engram or item per line), so check
before rerunning with `--replace-all`, which keeps one line and saves the old
file as `GameUserSettings.ini.bak`. Details:
[set ini](../reference/cli-commands.md#set-ini).

## Running server

`edit ini` and `set ini` refuse while the instance runs: ARK rewrites its INI
files when it stops, which would undo a change made while it ran. `--restart`
stops the instance (saving the world), makes the change and starts it again:

```sh
ark-cli edit ini --instance my-server --file gameusersettings --restart
```

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
