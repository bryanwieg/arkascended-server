# SteamCMD in a throwaway container: build.sh runs download.sh in it, so the
# machine running the build needs only Docker.
# Pinned so a new Ubuntu release can't change the downloader. Bump the digest deliberately.
FROM ubuntu:24.04@sha256:562111b88caa79c71391c0c4d3c2114a7d5c35915c20561fd893cb1e07929e6b
RUN echo steam steam/question select "I AGREE" | debconf-set-selections \
    && echo steam steam/license note '' | debconf-set-selections
RUN set -eux; \
    dpkg --add-architecture i386; \
    apt-get update; \
    apt-get install -y --no-install-recommends software-properties-common; \
    add-apt-repository multiverse; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates steamcmd; \
    rm -rf /var/lib/apt/lists/*; \
    ln -s /usr/games/steamcmd /usr/bin/steamcmd
# The runtime image's home: Steam records its client path in libraryfolders.vdf.
# World-writable because build.sh may run this as the caller's UID.
RUN install -d -m 1777 /home/ark-user
ENV HOME=/home/ark-user
