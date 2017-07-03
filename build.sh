#!/bin/bash -e
docker build -t avamon-bot-build -f Dockerfile.build .
docker create --name avamon-bot-build avamon-bot-build
docker cp avamon-bot-build:/go/bin/avamon-bot ./avamon-bot
docker rm avamon-bot-build
docker rmi avamon-bot-build
docker build -t avamon-bot "$@" .
rm avamon-bot
