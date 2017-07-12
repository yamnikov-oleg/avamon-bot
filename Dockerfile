FROM alpine:latest

RUN apk add --no-cache ca-certificates

COPY avamon-bot /bin
COPY frontend/telegrambot/bin/avamon-bot/config.default.toml /var/avamon-bot/config.toml

CMD avamon-bot -config /var/avamon-bot/config.toml
