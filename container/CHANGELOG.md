# Changelog

## 0.2.0 (2026-11-16)


### Features

* **container:** add a build-time generator for the default GameUserSettings.ini
* **container:** add emit_warning and fixed severity prefixes to _logging
* **container:** bake the default GameUserSettings.ini into the image
* **container:** build from the published server files image
* **container:** install the baked default INI on first boot, remove config init


### Bug Fixes

* **container:** keep stdout clean when a command is passed to the container
* **container:** log each RCON reply once in the stop script, drop RCON_QUIET_MODE
* **container:** report a non-numeric RCON_PORT as an RCON error
* **container:** restore into a staging directory, then swap it into place
* **container:** stop doubling backup and restore lines in docker logs

## 0.1.1 (2026-11-15)


### Bug Fixes

* **container:** replace rcon-cli with an in-process RCON client

## 0.1.0 (2026-11-14)


### Bug Fixes

* container shutdown, backup and entrypoint low-hanging fixes
* **container:** report when the OOM killer kills the server
* **container:** warn on low host RAM instead of refusing to start
* keep the shared cluster volume out of instance backups and restores
* re-apply RCON and password settings from env on every boot
* refuse to start when no volume is mounted at Saved/
* trustworthy stop-save confirmation; no unsaved backups of live servers
