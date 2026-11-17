# Changelog

## [0.3.1](https://github.com/bryanwieg/arkascended-server/compare/cli-v0.3.0...cli-v0.3.1) (2026-11-17)


### Image Build

* move to Go 1.27 ([#13](https://github.com/bryanwieg/arkascended-server/issues/13)) ([0bf396c](https://github.com/bryanwieg/arkascended-server/commit/0bf396c7d8efb1c9ed1009cd53bdc50902b8eee9))

## 0.3.0 (2026-11-16)


### Features

* **cli:** remove superseded ARK images after update and start
* **container:** build from the published server files image


### Bug Fixes

* **cli:** format compose port targets with strconv, not fmt.Fprintf
* **cli:** refuse to edit GameUserSettings.ini before it exists
* **cli:** show image build output, and fix tests that ran a stale ark-cli

## 0.2.1 (2026-11-15)


### Bug Fixes

* **cli:** edit only changed compose lines instead of re-marshaling the project
* **cli:** register listen port's signal handler before opening listeners
* **cli:** take a new instance's passwords from the restored backup
* **container:** replace rcon-cli with an in-process RCON client

## 0.2.0 (2026-11-14)


### Features

* publish RCON on 127.0.0.1 by default; add enable/disable remote-rcon


### Bug Fixes

* **cli:** drop the host RAM check from start instance
* container shutdown, backup and entrypoint low-hanging fixes
* trustworthy stop-save confirmation; no unsaved backups of live servers

## 0.1.0 (2026-11-13)


### Features

* **cli:** add --version and point remote updates at Docker Hub
