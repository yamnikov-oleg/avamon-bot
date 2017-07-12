FROM golang:1.8

COPY . /go/src/github.com/yamnikov-oleg/avamon-bot

RUN go build -o /go/bin/avamon-bot \
    --ldflags '-extldflags "-static"' \
    github.com/yamnikov-oleg/avamon-bot/frontend/telegrambot/bin/avamon-bot
