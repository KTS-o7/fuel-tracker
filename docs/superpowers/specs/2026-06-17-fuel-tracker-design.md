# Fuel Tracker — Design Spec

## Overview

A single-binary Go web app for logging bike fuel purchases, tracking odometer readings, and viewing monthly dashboards. Runs at `fuel.shenthar.me` behind nginx basic auth. Exposes a JSON API for agent use.

## Stack

- **Language:** Go
- **Router:** chi
- **Database:** SQLite (mattn/go-sqlite3 via modernc.org/sqlite — pure Go, no CGO)
- **Frontend:** Vanilla HTML + CSS + JS (embedded via `go:embed`), single-page app, mobile-friendly responsive layout
- **Theme:** Light theme only — clean dashboard aesthetic, no dark mode
- **Auth:** nginx basic auth for web UI; API key header for programmatic access
- **Deployment:** Single binary behind nginx reverse proxy on VPS

## Data Model

```sql
CREATE TABLE entries (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    date          TEXT    NOT NULL,                -- ISO 8601 (2026-06-17)
    odometer      REAL    NOT NULL,                -- km, 2dp
    liters        REAL    NOT NULL,                -- 2dp
    price_per_l   REAL    NOT NULL,                -- INR, 2dp
    fuel_type     TEXT    NOT NULL DEFAULT 'regular', -- 'regular' | 'premium'
    notes         TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL                 -- ISO 8601 timestamp
);
```

**Computed fields (not stored):**
- `total_cost = liters * price_per_l`
- `kmpl = (odo_n - odo_n-1) / liters_n` (calculated at query time between consecutive fills)

## API Endpoints

All endpoints require `Authorization: Bearer <API_KEY>` header for programmatic access.

### Entries

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/entries` | List all entries, newest first. Optional `?month=2026-06` |
| `POST` | `/api/entries` | Create entry |
| `GET` | `/api/entries/{id}` | Get single entry |
| `PUT` | `/api/entries/{id}` | Update entry |
| `DELETE` | `/api/entries/{id}` | Delete entry |

**POST/PUT body:**
```json
{
  "date": "2026-06-17",
  "odometer": 620.0,
  "liters": 11.0,
  "price_per_l": 110.89,
  "fuel_type": "regular",
  "notes": ""
}
```

### Stats

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/stats?month=2026-06` | Monthly summary |

**Response:**
```json
{
  "month": "2026-06",
  "total_km": 245.5,
  "total_cost": 1219.79,
  "total_liters": 11.0,
  "avg_kmpl": 22.3,
  "fill_count": 3
}
```

### Export

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/export?from=2026-01-01&to=2026-06-17` | CSV download |

Returns `Content-Type: text/csv` with `Content-Disposition: attachment`.

## Frontend Pages

Single HTML file, tab-based SPA. Fully responsive — works on mobile and desktop. Light theme only, clean dashboard aesthetic (white/gray cards, subtle borders, no beige).

Three tabs:

### Add
- Form: date (default today), odometer, liters, price per liter (default ~110.89), fuel type (regular/premium toggle), notes (optional)
- Save → POST /api/entries → redirect to History

### History
- Scrollable table with columns: date, odo, liters, price/L, total cost, fuel type, kmpl, notes, actions
- Sortable by column click
- Click row → edit modal (pre-filled add form)
- Delete button with confirmation
- Export buttons: Lifetime CSV, This Month CSV, Date Range CSV

### Dashboard
- Month selector (prev/next arrows, current month display)
- Summary cards: Total km, Total cost, Total liters, Avg kmpl, Fill count
- Simple canvas line chart: kmpl per fill across the month

## State on First Load

User starts with odometer ~620 km, ~11L in tank (13L capacity). First entry creates a baseline — no kmpl calculated until second fill. App shows "Need one more fill to calculate mileage" placeholder.

## Deployment

Build, deploy to VPS, configure nginx and SSL, then notify user to enable Cloudflare proxy.

### Build & Deploy

```
git push
ssh VPS
cd /opt/fuel-tracker/
git pull
go build -o fuel-tracker .
systemctl restart fuel-tracker   # or supervisor restart
```

### Directory Layout (VPS)

```
/opt/fuel-tracker/
├── fuel-tracker        # Compiled binary
├── data/
│   └── fuel.db         # SQLite (auto-created)
├── .env                # API_KEY, PORT
└── fuel-tracker.service  # systemd unit
```

### Nginx Config

Create `/etc/nginx/sites-enabled/fuel.shenthar.me`:
- nginx basic auth (htpasswd, bcrypt)
- Proxy to `127.0.0.1:{PORT}`
- SSL via Cloudflare (DNS-only initially for cert issuance)

### SSL & Cloudflare Process

1. Create nginx config with DNS-only Cloudflare
2. Run `certbot --nginx -d fuel.shenthar.me` to get SSL cert
3. **Notify user**: "SSL certificate issued. Enable the orange cloud (proxy) on the DNS record for `fuel.shenthar.me` in Cloudflare dashboard now."
4. Wait for user confirmation before continuing

## Future Considerations (not building now)
- Multi-bike support
- Service reminders based on odometer
- Push notifications for fuel price drops
