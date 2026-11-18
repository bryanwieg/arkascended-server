# Changelog

## [0.4.0](https://github.com/bryanwieg/arkascended-server/compare/cli-v0.3.2...cli-v0.4.0) (2026-11-18)


### Features

* back up and restore shared cluster data (--instance cluster) ([#27](https://github.com/bryanwieg/arkascended-server/issues/27)) ([64d3d63](https://github.com/bryanwieg/arkascended-server/commit/64d3d6307bd03fde2da3b02f8e2a5ea02a4e4529))
* join a server cluster with CLUSTER_ID ([#29](https://github.com/bryanwieg/arkascended-server/issues/29)) ([82d0012](https://github.com/bryanwieg/arkascended-server/commit/82d001246719866d7867c6cb8474912259065c42))
* load CurseForge mods with MOD_IDS ([#28](https://github.com/bryanwieg/arkascended-server/issues/28)) ([b002f3c](https://github.com/bryanwieg/arkascended-server/commit/b002f3c9bd81e0ebbdf169c9599e9baa22a303e7))


### Bug Fixes

* **update:** keep every tag of an image a container uses ([#31](https://github.com/bryanwieg/arkascended-server/issues/31)) ([292d3fc](https://github.com/bryanwieg/arkascended-server/commit/292d3fcc13782bbdea252b5a2febcd31ff46fac7))
* **update:** rebuild the image when the embedded recipe changes ([#30](https://github.com/bryanwieg/arkascended-server/issues/30)) ([07efe78](https://github.com/bryanwieg/arkascended-server/commit/07efe78aad623f7c8b71d9e434e815e49359f155))


### Dependencies

* bump the go-security group across 1 directory with 2 updates ([#25](https://github.com/bryanwieg/arkascended-server/issues/25)) ([8563842](https://github.com/bryanwieg/arkascended-server/commit/8563842cf1eb02802d9ff4a0bd97f591699ca212))

## [0.3.2](https://github.com/bryanwieg/arkascended-server/compare/cli-v0.3.1...cli-v0.3.2) (2026-11-17)


### Dependencies

* bump the go-security group across 1 directory with 10 updates ([#18](https://github.com/bryanwieg/arkascended-server/issues/18)) ([146044d](https://github.com/bryanwieg/arkascended-server/commit/146044da5b26402c37b72f990b28235b7dd5ee1e))

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
