FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# System deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv \
    build-essential git curl wget ca-certificates \
    postgresql-client-common postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Go 1.26
ARG GOARCH=arm64
RUN wget -q "https://go.dev/dl/go1.26.1.linux-${GOARCH}.tar.gz" -O /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"
ENV GOPATH="/root/go"

WORKDIR /app
COPY . /app/

RUN chmod +x /app/test-pipeline.sh /app/scripts/*

CMD ["/app/test-pipeline.sh"]
