# Multi-stage build для минимального размера
FROM golang:1.24-alpine AS builder

# Разрешить автоматическую загрузку нужной версии Go
ENV GOTOOLCHAIN=auto

# Установить зависимости для сборки
RUN apk add --no-cache git make ca-certificates

WORKDIR /build

# Скопировать go.mod и go.sum
COPY go.mod go.sum ./
RUN go mod download

# Скопировать исходный код
COPY . .

# Собрать бинарник с release/commit/build metadata.
RUN set -eu; \
    CTS_CORE_VERSION="$(cat VERSION)"; \
    CTS_CORE_COMMIT="$(git rev-parse --short HEAD)"; \
    CTS_CORE_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; \
    if CTS_CORE_RELEASE="$(git describe --tags --exact-match 2>/dev/null)"; then \
        CTS_CORE_RELEASE="$(echo "${CTS_CORE_RELEASE}" | tr -d '[:space:]')"; \
    else \
        BASE_TAG="$(git describe --tags --abbrev=0 2>/dev/null || true)"; \
        if [ -z "${BASE_TAG}" ]; then \
            CTS_CORE_RELEASE="${CTS_CORE_VERSION}"; \
        else \
            COMMITS_SINCE_TAG="$(git rev-list --count "${BASE_TAG}"..HEAD)"; \
            BUILD_STAMP="$(date -u +%Y%m%d%H%M%S)"; \
            CTS_CORE_RELEASE="${BASE_TAG}-dev.${COMMITS_SINCE_TAG}+${BUILD_STAMP}.${CTS_CORE_COMMIT}"; \
        fi; \
    fi; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${CTS_CORE_VERSION} -X main.release=${CTS_CORE_RELEASE} -X main.commit=${CTS_CORE_COMMIT} -X main.buildTime=${CTS_CORE_BUILD_TIME}" \
    -o cts-core \
    cmd/cts-core/main.go

# Final stage - минимальный образ
FROM alpine:latest

# Установить ca-certificates для HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Создать non-root пользователя
#RUN addgroup -S ctscore && adduser -S ctscore -G ctscore

WORKDIR /app

# Скопировать бинарник из builder stage
COPY --from=builder /build/cts-core .

# Скопировать примеры конфигов (реальные будут через volume)
#COPY --chown=ctscore:ctscore conf/config.example.yaml ./conf/

# Создать необходимые директории
#RUN mkdir -p logs state pki && \
#    chown -R ctscore:ctscore logs state pki

# Переключиться на non-root пользователя
#USER ctscore

# Expose порт для REST API (Phase 1.5)
EXPOSE 8080

# Запуск приложения
ENTRYPOINT ["./cts-core"]
CMD ["-config", "conf/config.yaml"]
