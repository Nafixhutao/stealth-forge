# The Document Agent sandbox image. One container per run; the runner starts it
# with a read-only root filesystem, no network, one writable /workspace bind
# mount, a non-root user, and explicit CPU/memory/process limits.
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        python3 \
        python3-pip \
        libreoffice-writer \
        libreoffice-calc \
        fonts-dejavu-core \
        fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --break-system-packages python-docx

WORKDIR /workspace
USER 65532:65532
