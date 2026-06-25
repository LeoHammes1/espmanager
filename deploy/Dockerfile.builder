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
    && pip install --no-cache-dir platformio
WORKDIR /app
COPY --from=build /out/espmanager-builder /usr/local/bin/espmanager-builder
ENV ESPM_BUILD_WORKSPACE=/app/data
ENTRYPOINT ["espmanager-builder"]
