# Lite test image: the real runtime scripts at the real paths, a fake ARK server behind a fake
# proton, and no ARK files. For tests that exercise the scripts and the CLI without a real server.
# Built by tests/helpers/lite_image.py from a staged context; see docs/dev/reference/test-architecture.md.
FROM python:3.14-slim

# As in the real image: zstd (backup and restore run tar --zstd), git (the INI history).
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends zstd git; \
    rm -rf /var/lib/apt/lists/*; \
    groupadd -g 3000 ark-user; \
    useradd -l -m -d /home/ark-user -s /bin/bash -u 3000 -g 3000 ark-user; \
    ln -s /usr/local/bin/python3.14 /usr/bin/python3.14

USER ark-user
ENV HOME=/home/ark-user
WORKDIR /home/ark-user
RUN mkdir -p ark-server/ShooterGame/Saved/Logs ark-server/ShooterGame/Saved/clusters \
    ark-server/ShooterGame/Binaries/Win64 maintenance/output

ENV PROTON_PATH=/home/ark-user/fake-proton/
COPY --chown=ark-user:ark-user --chmod=0755 proton fake_ark_server.py /home/ark-user/fake-proton/
COPY --chown=ark-user:ark-user GameUserSettings.ini /home/ark-user/config-defaults/GameUserSettings.ini
COPY --chown=ark-user:ark-user --chmod=0755 runtime /home/ark-user/scripts/runtime

ENTRYPOINT ["/home/ark-user/scripts/runtime/container-entrypoint.sh"]
