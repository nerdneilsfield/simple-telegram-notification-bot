# Telegram Notification Server

This Golang application serves as a Telegram bot server that allows users to subscribe and receive messages sent through different endpoints. The server leverages the Gin web framework for handling HTTP requests, and the Telegram Bot API for interacting with Telegram.

## Features

- Subscription management via Telegram commands (`/subscribe`, `/unsubscribe`, `/regenerate`, `/info`, `/help`).
- Generating unique UUID and AES key for each subscriber.
- Encrypted message support using AES encryption.
- Different endpoints for sending messages or files to a subscribed Telegram user.
- Database storage of subscription records using SQLite and GORM.
- Structured logging using Uber's Zap logging library.

## Dependencies

- Gin Web Framework: [github.com/gin-gonic/gin](https://github.com/gin-gonic/gin)
- Telegram Bot API: [github.com/go-telegram-bot-api/telegram-bot-api/v5](https://github.com/go-telegram-bot-api/telegram-bot-api/v5)
- GORM: [gorm.io/gorm](https://gorm.io/gorm)
- SQLite Driver for GORM: [gorm.io/driver/sqlite](https://gorm.io/driver/sqlite)
- TOML Parser: [github.com/BurntSushi/toml](https://github.com/BurntSushi/toml)
- Zap Logging: [go.uber.org/zap](https://go.uber.org/zap)
- Google UUID: [github.com/google/uuid](https://github.com/google/uuid)

## Configuration

Configuration is done through a TOML file specified by the `-conf` flag (default: `config.toml`). The configuration file contains the following fields:

- `telegram_token`: Your Telegram Bot token.
- `telegram_api_url`: The URL of the Telegram API.
- `gin_address`: The address and port on which the Gin server should listen.
- `post_url`: The base URL for POSTing messages.

Database path is specified by the `-db` flag (default: `subscriptions.db`).

Example `config.toml`:

```toml
telegram_token = "your-telegram-bot-token"
telegram_api_url = "https://api.telegram.org/bot%s/%s"
gin_address = ":8080"
post_url = "http://example.com"
```

## Usage

1. Compile and run the server.
2. Interact with the Telegram bot to subscribe and receive a UUID and AES key.
3. Use the provided endpoints to send messages or files to the subscribed Telegram user.

The following endpoints are available for interacting with the server:

- POST `/api/:uuid/json`: Send a JSON payload with an optional encrypted message.
- GET `/api/:uuid/get`: Send a message via query parameters.
- POST `/api/:uuid/form`: Send a message via form data.
- POST `/api/:uuid/file`: Send a file via form data.

### Building

To build the project, ensure you have Go installed and run:

```bash
go mod download
go build
```

This will generate an executable file in the project directory.

### Running

```
cp confg.example.toml config.toml
```

Modify the `config.toml` and put your configurations.

```
./server -conf config.toml -db subscriptions.db
```

### Using Docker

We provide a `Dockerfile` to run application, you can use with prebuilt docker `nerdneils/simple-telegram-notification-bot:latest` .

```
docker run -it --rm -p 7888:7888 -v ${PWD}/config.toml:/etc/notification-bot/config.toml -v ${PWD}/data:/data nerdneils/simple-telegram-notification-bot:latest  ./server -conf /etc/notification-bot/config.toml -db /data/notification-bot.db
```

You can also use the docker-compose to deoply your application, we provide a example `docker-compose.yml`, which use Traefik as the API proxy:

```yaml
version: '3.3'

networks:
  proxy:
    external: true

services:
  notification-bot:
    image: nerdneils/simple-telegram-notification-bot:latest
    networks:
      - proxy
    command: ./server -conf /etc/notification-bot/config.toml -db /data/notification-bot.db
    volumes:
      - ./config.toml:/etc/notification-bot/config.toml
      - ./data:/data
    labels:
      - traefik.enable=true
      - traefik.docker.network=proxy
      - "traefik.http.routers.tg-notification.entrypoints=http"
      - "traefik.http.routers.tg-notification.rule=Host(`tg-notification.example.org`)"
      - "traefik.http.middlewares.https-redirect.redirectscheme.scheme=https"
      - "traefik.http.routers.tg-notification.middlewares=https-redirect@docker"
      - traefik.http.routers.tg-notification.entrypoints=https
      - "traefik.http.routers.tg-notification.rule=Host(`tg-notification.example.org`)"
      - traefik.http.routers.tg-notification.tls=true
      - traefik.http.routers.tg-notification.tls.certresolver=cloudflare
      - "traefik.http.services.tg-notification.loadbalancer.server.port=7888"
```

