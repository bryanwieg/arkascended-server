# Changelog

## [0.7.0](https://github.com/bryanwieg/arkascended-server/compare/cli-v0.6.0...cli-v0.7.0) (2026-11-22)


### ⚠ BREAKING CHANGES

* **cli:** a compose service without the ark.port.asa label is no longer managed by ark-cli. Move an older instance (game port published as <port>:27015/udp) to <port>:<port>/udp with ASA_PORT=<port> and the label ark.port.asa: "<port>".

### Bug Fixes

* **cli:** backlog bug fixes; instances need the ark.port.asa label ([#45](https://github.com/bryanwieg/arkascended-server/issues/45)) ([c83b248](https://github.com/bryanwieg/arkascended-server/commit/c83b248c98699bc7832e10cd95cb94fb158d678f))

## [0.6.0](https://github.com/bryanwieg/arkascended-server/compare/cli-v0.5.0...cli-v0.6.0) (2026-11-22)


### ⚠ BREAKING CHANGES

* **scheduler:** compose scheduler service with health monitor, replacing host cron ([#42](https://github.com/bryanwieg/arkascended-server/issues/42))

### Features

* **scheduler:** compose scheduler service with health monitor, replacing host cron ([#42](https://github.com/bryanwieg/arkascended-server/issues/42)) ([4a2a3fc](https://github.com/bryanwieg/arkascended-server/commit/4a2a3fcdd4b9467993aad2e86b7fc3ebf4f99ca0))


### Bug Fixes

* **stop:** no retry delay after the last save attempt ([#40](https://github.com/bryanwieg/arkascended-server/issues/40)) ([05bde9b](https://github.com/bryanwieg/arkascended-server/commit/05bde9bb9af9b2aac8e06c5dececae938a68dba1))
* **stop:** shorter stop timeouts ([#39](https://github.com/bryanwieg/arkascended-server/issues/39)) ([ddd6fb3](https://github.com/bryanwieg/arkascended-server/commit/ddd6fb3f8ece6ec58cf2aa386770f9d288111047))

## [0.5.0](https://github.com/bryanwieg/arkascended-server/compare/cli-v0.4.0...cli-v0.5.0) (2026-11-19)


### ⚠ BREAKING CHANGES

* **cli:** `edit ini --force` is removed. A running instance needs `--restart`; editing a live instance is no longer possible.

### Features

* add `list players` to show connected players via RCON ([715f760](https://github.com/bryanwieg/arkascended-server/commit/715f760698d49bae4c49bc5bf2eacf295fe7ce87))
* **cli:** add `get ini` to print one INI key ([33e7bf6](https://github.com/bryanwieg/arkascended-server/commit/33e7bf64db43824219abeb56b5a2495fe8ece870))
* **cli:** add `set ini` to change one INI key without an editor ([9c4ff27](https://github.com/bryanwieg/arkascended-server/commit/9c4ff27f3f8e5babe711b523bd5eeb295fcee631))
* **container:** add ark-ini.py to read and set INI keys inside the container ([83cd1b9](https://github.com/bryanwieg/arkascended-server/commit/83cd1b9a172c9bb1c5149e690c525e7fca5259ad))
* set ini and get ini through ark-ini.py, INI git history, lite test image ([#36](https://github.com/bryanwieg/arkascended-server/issues/36)) ([175189e](https://github.com/bryanwieg/arkascended-server/commit/175189e660de99728b06c63227612da9be8cfd8f))
* **stop:** player-aware shutdown countdown naming the reason ([3f0bdc2](https://github.com/bryanwieg/arkascended-server/commit/3f0bdc2cb7c41652c8d319cc0fe147eaa6e26ec2))
* **stop:** warn online players for 5 minutes by default ([#38](https://github.com/bryanwieg/arkascended-server/issues/38)) ([f5d0c5e](https://github.com/bryanwieg/arkascended-server/commit/f5d0c5ef6dc1084a0a4cf98d516aca424d93eb12))
* **update:** log each update step; name a failed build's cause ([b15be25](https://github.com/bryanwieg/arkascended-server/commit/b15be256edf144df220abe1f546be7a20e2bf875))
* **update:** report ARK versions; --check targets Steam's build ([0d95e45](https://github.com/bryanwieg/arkascended-server/commit/0d95e45954d41e449e525995f58120ddd2d723a8))
* **update:** versions, step logging, player countdown; fix SteamCMD 0x6 ([#37](https://github.com/bryanwieg/arkascended-server/issues/37)) ([4017116](https://github.com/bryanwieg/arkascended-server/commit/4017116d9e54cdbba8a68d014d2114777c367a61))


### Bug Fixes

* ark-cli 0.4.0 bug batch, plus `list players` ([#33](https://github.com/bryanwieg/arkascended-server/issues/33)) ([a86503a](https://github.com/bryanwieg/arkascended-server/commit/a86503a0829caf6887a546d1d286fb311638abe2))
* **backup:** timestamp every archive and lock prune and restore ([0db3ada](https://github.com/bryanwieg/arkascended-server/commit/0db3ada8bf4dc4a79d2b228a871504a9f855ee24))
* **build:** fall back to a validate install on SteamCMD state 0x6 ([0543bcf](https://github.com/bryanwieg/arkascended-server/commit/0543bcf337b247924a8241bd8e53a5814b83a32e))
* **build:** keep SteamCMD's depot manifests between builds ([fd708de](https://github.com/bryanwieg/arkascended-server/commit/fd708dedde9d78cfbede8d2c40587c22acd25670))
* **cli:** correct help text for five commands ([3653585](https://github.com/bryanwieg/arkascended-server/commit/36535852a915e2bc565a85288a74db40b4222d10))
* **cli:** edit ini on a running instance stops it before editing ([1bb0281](https://github.com/bryanwieg/arkascended-server/commit/1bb028108eab728f5fa3868ed4adf5da4e8bca8d))
* **health:** read readiness logs from the container's current start ([27fe18c](https://github.com/bryanwieg/arkascended-server/commit/27fe18c199be84b5897c0cebafa78dfc45d00a49))
* **image:** tag CI-built images with the recipe ID like ark-cli ([b8aa375](https://github.com/bryanwieg/arkascended-server/commit/b8aa3757358cc748759980565de27265b23b2c2f))
* **reachability:** correct --listen hints and remote UDP status ([940a843](https://github.com/bryanwieg/arkascended-server/commit/940a843215809055059949577c7c73b636783976))
* **reconfigure:** clear values with "" and follow a custom RCON_PORT ([af59ade](https://github.com/bryanwieg/arkascended-server/commit/af59ade7251846848b25062fcf6758546f797531))
* stop with the default grace period on restart, rollout and teardown ([5b3e2d1](https://github.com/bryanwieg/arkascended-server/commit/5b3e2d1db940ab43f2ae02b9010d538bd961f929))
* **tests:** container_ensure_image_tag called a method that doesn't exist ([009bdda](https://github.com/bryanwieg/arkascended-server/commit/009bdda59e003934afd20541daff44ce1c7a179f))
* **tests:** tag the rollback test's broken image with the recipe ID ([4357bbb](https://github.com/bryanwieg/arkascended-server/commit/4357bbb4d42046a28ab8bad2686fd56e99612528))
* **tests:** wait for the server to start before the unresponsive-RCON stop ([13d2174](https://github.com/bryanwieg/arkascended-server/commit/13d21749c471a10cd8699157863b7b9e9f659fd8))

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
