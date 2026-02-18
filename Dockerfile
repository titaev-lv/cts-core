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

# Собрать бинарник
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
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
