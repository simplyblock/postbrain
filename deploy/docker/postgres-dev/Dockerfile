FROM pgvector/pgvector:pg18 AS age-builder

ARG AGE_GIT_REF=master

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        bison \
        ca-certificates \
        flex \
        git \
        postgresql-server-dev-18 \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 --branch "${AGE_GIT_REF}" https://github.com/apache/age.git /tmp/age \
    && make -C /tmp/age PG_CONFIG=/usr/lib/postgresql/18/bin/pg_config \
    && make -C /tmp/age PG_CONFIG=/usr/lib/postgresql/18/bin/pg_config install DESTDIR=/tmp/age-install

FROM pgvector/pgvector:pg18

COPY --from=age-builder /tmp/age-install/ /
