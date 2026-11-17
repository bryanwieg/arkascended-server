# Edit Server INI Files

How to edit `Game.ini` and `GameUserSettings.ini` for your ARK server.

The CLI edits INI files inside the container so volume file ownership stays
consistent. Do not edit them directly on the host while the container is
running.

## Open an INI file for editing

```sh
ark-cli edit ini --instance my-server --file gameusersettings
ark-cli edit ini --instance my-server --file game
```

`GameUserSettings.ini` must already exist: the instance installs ARK's full
default file on its first start. Before that, the command refuses ("doesn't
exist yet. Start the instance once…") rather than create a partial file that
would stop the defaults from being installed. `Game.ini` is different: ARK
never creates it, so editing it creates it.

This opens the file in `nano` by default. Use a different editor with
`--editor`:

```sh
ark-cli edit ini --instance my-server --file game --editor vi
ark-cli edit ini --instance my-server --file gameusersettings --editor vim
```

## Settings managed from the environment

Four `[ServerSettings]` keys in `GameUserSettings.ini` are rewritten from the
instance's environment every time the container starts: `RCONEnabled`,
`RCONPort`, `ServerAdminPassword` and `ServerPassword`. Edits to them in the
INI are lost on restart. Change them with `ark-cli reconfigure instance`
(`--admin-password`, `--server-password`) instead.

## Edit while the server is running

By default the command refuses to edit if the server is running, to prevent
config changes from being overwritten by the live process.

To edit anyway (server must be restarted for changes to take effect):

```sh
ark-cli edit ini --instance my-server --file gameusersettings --force
```

## Edit and restart in one step

If the server is already running and you want to apply changes immediately:

```sh
ark-cli edit ini --instance my-server --file gameusersettings --force --restart
```

This opens the editor. When you save and exit, the instance is automatically
restarted.

## After editing (without --restart)

Changes to INI files take effect on the next server start. If the server is
currently running, restart it:

```sh
ark-cli restart instance --instance my-server
```

## Where are the INI files?

The INI files live in the instance's Docker volume under:
```
ShooterGame/Saved/Config/WindowsServer/Game.ini
ShooterGame/Saved/Config/WindowsServer/GameUserSettings.ini
```

To find the volume's location on the host:

```sh
ark-cli show volume-paths --instance my-server
```
