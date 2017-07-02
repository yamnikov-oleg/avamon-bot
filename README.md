# avamon-bot

Small telegram bot monitoring web sites availability.

## Building and running

### With Docker

You must have docker installed to use this method.

1. Clone the repo.
2. Run `build.sh -t avamon-bot`, it will build the docker image for you.
3. Copy the default config at `./frontend/avamon-bot/config.default.toml` and
  edit it however you like. Don't forget to specify the bot token.
4. Run the redis container like so:
  `docker run -p 6379:6379 redis`
5. Run the bot container this way:
  `docker run -v ./path/to/config.toml:/var/avamon-bot/config.toml avamon-bot`
6. That's it.

### With Go compiler.

1. Build and install it: `go get github.com/yamnikov-oleg/avamon-bot`
2. Copy and edit the config file at `./frontend/avamon-bot/config.default.toml`.
  Don't forget to specify the bot token.
3. Run it like so: `avamon-bot -config ./path/to/config.toml`
