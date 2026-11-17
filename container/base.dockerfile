# Base image: everything in the ARK server image that is the same for every ARK build
# and every host (packages, Proton, XAudio, the ark-user account). It holds nothing of
# Studio Wildcard's, so it is published to ghcr.io/bryanwieg/ark-server-base, tagged by
# the first 12 hex characters of this file's sha256. Never COPY from the build context
# here: the tag must cover everything that goes into the image.
#
# Bumping the refresh line rebuilds the base with current Ubuntu packages
# (.github/workflows/base-refresh.yml does it monthly).
# refresh: 2026-11

ARG PROTON_VERSION=11-12
# Build-only stages are pinned so a new Ubuntu release can't invalidate their cached
# downloads. The final stage tracks ubuntu:24.04 so it picks up security updates
# whenever the base is rebuilt. Bump the digest deliberately.
ARG BUILD_BASE_IMAGE=ubuntu:24.04@sha256:2198e4c65076e9cb0b3d695219df667c0e3b0072da64ca3c2648aca17dcecaf5

# One small stage per downloaded tool, so each download reruns only when its own
# version changes.
FROM ${BUILD_BASE_IMAGE} AS fetch_base
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl unzip; \
    rm -rf /var/lib/apt/lists/*

FROM fetch_base AS proton
ARG PROTON_VERSION
RUN set -eux; \
    mkdir -p "/out/GE-Proton${PROTON_VERSION}"; \
    curl -fL "https://github.com/GloriousEggroll/proton-ge-custom/releases/download/GE-Proton${PROTON_VERSION}/GE-Proton${PROTON_VERSION}-x86_64.tar.gz" -o /tmp/proton.tar.gz; \
    # GE-Proton 11+ archives unpack to GE-Proton<ver>-x86_64/; strip it so runtime paths stay GE-Proton<ver>/.
    tar -xf /tmp/proton.tar.gz -C "/out/GE-Proton${PROTON_VERSION}" --strip-components=1; \
    rm /tmp/proton.tar.gz

FROM fetch_base AS xaudio
RUN set -eux; \
    curl -fL "https://globalcdn.nuget.org/packages/microsoft.xaudio2.redist.1.2.13.nupkg?packageVersion=1.2.13" -o /tmp/xaudio.zip; \
    unzip /tmp/xaudio.zip -d /root/microsoft.xaudio2.redist

# The base itself: layers are ordered from rarely changing to often changing, so a
# change rebuilds (and users re-pull) only the layers after it.
FROM ubuntu:24.04

RUN set -eux; \
    dpkg --add-architecture i386; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
    software-properties-common; \
    add-apt-repository multiverse; \
    add-apt-repository ppa:deadsnakes/ppa; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
    python3.14 \
    libfreetype6:i386 \
    libfreetype6 \
    libimage-exiftool-perl \
    libcurl4 \
    libvulkan1 \
    ca-certificates \
    locales \
    btop \
    zstd \
    nano; \
    rm -rf /var/lib/apt/lists/*

RUN sed -i 's/^# *\(en_US.UTF-8\)/\1/' /etc/locale.gen \
    && locale-gen en_US.UTF-8
ENV LANG='en_US.UTF-8'
ENV LANGUAGE='en_US:en'

RUN set -eux; \
    groupadd -g 3000 ark-user; \
    useradd -l -m -d /home/ark-user -s /bin/bash -u 3000 -g 3000 ark-user; \
    cat /proc/sys/kernel/random/uuid > /etc/machine-id

USER ark-user
ENV HOME=/home/ark-user
WORKDIR /home/ark-user

RUN set -eux; \
    mkdir -p /home/ark-user/.config/protonfixes; \
    mkdir -p /home/ark-user/ark-server/ShooterGame/Saved/Logs; \
    mkdir -p /home/ark-user/ark-server/ShooterGame/Saved/clusters; \
    mkdir -p /home/ark-user/maintenance/output

ENV PROTON_NO_ESYNC=0
ENV PROTON_NO_FSYNC=0
ENV WINEDEBUG=-all
ENV STEAM_COMPAT_CLIENT_INSTALL_PATH=/home/ark-user/.local/share/Steam

# Declared here, not at the top of the stage, so a Proton bump doesn't re-run the steps above.
ARG PROTON_VERSION
ENV PROTON_VERSION=${PROTON_VERSION}
ENV PROTON_PATH=/home/ark-user/.steam/compatibilitytools.d/GE-Proton${PROTON_VERSION}/
ENV STEAM_COMPAT_DATA_PATH=/home/ark-user/.steam/compatibilitytools.d/GE-Proton${PROTON_VERSION}/files/share/default_pfx
COPY --from=proton --chown=ark-user:ark-user /out/ /home/ark-user/.steam/compatibilitytools.d/

# Staged here; the runtime dockerfile copies it into the server after the ARK files.
COPY --from=xaudio --chown=ark-user:ark-user /root/microsoft.xaudio2.redist/build/native/release/bin/x64/xaudio2_9redist.dll /home/ark-user/redist/xaudio2_9redist.dll

LABEL org.opencontainers.image.source="https://github.com/bryanwieg/arkascended-server" \
      org.opencontainers.image.licenses="AGPL-3.0-only"
