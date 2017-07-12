FROM alpine:latest

RUN apk add --no-cache ca-certificates

COPY avamon-bot /bin
COPY frontend/telegrambot/config.default.toml /var/avamon-bot/config.toml

CMD avamon-bot -config /var/avamon-bot/config.toml
