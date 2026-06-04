FROM pgvector/pgvector:pg18-trixie

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ARG PARADEDB_VERSION=0.23.3
ARG PG_MAJOR=18
ENV DEBIAN_FRONTEND=noninteractive

RUN set -eux; \
  apt-get update -qq; \
  apt-get install -y --no-install-recommends ca-certificates curl; \
  ARCH="$(dpkg --print-architecture)"; \
  DEB="postgresql-${PG_MAJOR}-pg-search_${PARADEDB_VERSION}-1PARADEDB-trixie_${ARCH}.deb"; \
  URL="https://github.com/paradedb/paradedb/releases/download/v${PARADEDB_VERSION}/${DEB}"; \
  curl -fsSL "$URL" -o "/tmp/${DEB}"; \
  apt-get install -y --no-install-recommends "/tmp/${DEB}"; \
  apt-get clean; \
  rm -rf /var/lib/apt/lists/* "/tmp/${DEB}"
