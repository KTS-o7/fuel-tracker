# Fuel Tracker

Single-binary Go web app for logging bike fuel purchases, tracking odometer readings, and viewing monthly dashboards. Live at https://fuel.shenthar.me.

## Stack

- Go + chi + modernc.org/sqlite (pure Go, no CGO)
- Vanilla HTML/CSS/JS SPA, embedded via `go:embed`
- Login + bearer token authentication
- Single binary, light theme, mobile responsive

## Local development

```sh
cp .env.example .env  # edit ADMIN_USER and ADMIN_PASSWORD
go build -o fuel-tracker .
./fuel-tracker
# open http://localhost:9124
```

## Tests

```sh
go test ./...
```

## Deploy

See `deploy/` for the systemd unit and nginx config.