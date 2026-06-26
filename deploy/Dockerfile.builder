FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/espmanager-builder ./cmd/espmanager-builder

FROM python:3.12-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir platformio \
    && useradd -m -u 10001 builder
COPY --from=build /out/espmanager-builder /usr/local/bin/espmanager-builder
RUN mkdir -p /app/data /home/builder/.platformio \
    && chown -R builder:builder /app /home/builder/.platformio
WORKDIR /app
ENV ESPM_BUILD_WORKSPACE=/app/data \
    PLATFORMIO_CORE_DIR=/home/builder/.platformio \
    HOME=/home/builder
USER builder
ENTRYPOINT ["espmanager-builder"]
