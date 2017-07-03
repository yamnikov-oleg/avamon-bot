# avamon-bot

Small telegram bot monitoring web sites availability.

## Building and running

### With Docker

You must have docker installed to use this method.

1. Clone the repo.
2. Run `build.sh`, it will build the docker image for you and will name it `avamon-bot`.
3. Copy the default config from `./frontend/avamon-bot/config.default.toml` and
  edit it however you like. Don't forget to specify the bot token.
  Also set redis host to `redis`.
4. Create `docker-compose.yml` with this content:
  ```
  version: '3'
  services:
    bot:
      image: avamon-bot
      links:
      - redis
      volumes:
      - ./config.toml:/var/avamon-bot/config.toml:ro
    redis:
      image: redis
  ```
5. Run with `docker-compose up`.

If you want to persist the sqlite3 database, edit the path of the db file in
the config (`database.name`), then mount it as docker volume.

### With Go compiler.

1. Build and install it: `go get github.com/yamnikov-oleg/avamon-bot`
2. Copy and edit the config file at `./frontend/avamon-bot/config.default.toml`.
  Don't forget to specify the bot token.
3. Run it like so: `avamon-bot -config ./path/to/config.toml`
