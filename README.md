# crossletters

Self-hostable, turn-based, async word game. Single Go binary that serves the JSON
API and an embedded React/TypeScript frontend. Runs on Cloud Run + GCS in
production; also works fully offline with an in-memory store for local
development.

## Quick start (local)

You need Go 1.26+ and Node 20+.

```sh
# One-time: install frontend deps + build the embedded bundle
(cd web && npm install && npm run build)

# Run the server (serves API + built frontend on http://127.0.0.1:8080)
go run ./cmd/server
```

Open http://127.0.0.1:8080 in one browser, then open a second browser (or a
private window) for the other player. The dev-login form on the landing page
takes any `userId` + display name and sets a cookie. Create a game in one
window, copy the invite code into the other, and join — the game is active
immediately and players can start placing tiles as soon as they join.

### Dev loop with hot reload

Run the Go API and Vite dev server in two terminals. Vite proxies `/api` to
the Go server, so you get hot-module reload on the frontend without rebuilding.

```sh
# terminal 1
go run ./cmd/server

# terminal 2
cd web && npm run dev
# open http://127.0.0.1:5173
```

### Tests

```sh
go test ./...                # engine + store + http integration tests
(cd web && npm run typecheck)   # frontend typechecks
```

## Production (Cloud Run)

Infrastructure is managed by Terraform in `infra/`. One-time setup:

```sh
cd infra
terraform init
terraform apply   # requires terraform.tfvars — see below
```

This provisions the GCS bucket, Artifact Registry, service account, IAM
bindings, Secret Manager secrets, and Cloud Run service. Secrets
(`SESSION_SECRET`, `VAPID_PRIVATE_KEY`) are stored in Google Secret Manager
and injected into Cloud Run via secret references — they never appear as
plain-text env vars.

Create `infra/terraform.tfvars` (gitignored) with your values:

```hcl
google_client_id  = "YOUR_OAUTH_CLIENT_ID"
vapid_public_key  = "YOUR_VAPID_PUBLIC_KEY"
vapid_private_key = "YOUR_VAPID_PRIVATE_KEY"
vapid_contact     = "mailto:you@example.com"
```

To deploy (or redeploy after code changes):

```sh
gcloud run deploy not-scrabble --source=. --region=us-central1 --project=not-scrabble
```

The service scales to zero (`min-instances=0`) so idle cost is $0.

## Dictionary

On startup the server loads a word list from the `-dict` flag. If no file is
found it falls back to a tiny built-in list (~78 common words) — enough to
sanity-check a play flow but too small for real games.

Fetch ENABLE (public domain, 172,819 words) for a good default:

```sh
curl -o data/enable.txt https://norvig.com/ngrams/enable1.txt
```

Point `-dict` at any newline-separated word file:

```sh
go run ./cmd/server -dict data/enable.txt
# or the gzipped form
go run ./cmd/server -dict data/enable.txt.gz
```

Word list files under `data/` are `.gitignore`'d.

## Project layout

```
cmd/server/         # main.go — HTTP server entrypoint
internal/game/      # game engine (board, bag, rack, scoring, validation)
internal/dict/      # newline-separated word-list loader
internal/store/     # game/user persistence (in-memory + GCS backends)
internal/httpapi/   # HTTP handlers, auth (Google + dev), allowlist
internal/push/      # Web Push (VAPID) notification sender
web/                # React + TypeScript + Vite frontend
webdist/            # Go package that //go:embeds the built frontend
infra/              # Terraform for GCP (Cloud Run, GCS, IAM, Artifact Registry)
```

The Go binary embeds `webdist/dist/` at build time via `//go:embed`. A
`placeholder.txt` keeps the embed compiling on a fresh clone before you've run
`npm run build`. `vite.config.ts` uses `emptyOutDir: false` so the placeholder
survives rebuilds.

## Gameplay rules

Standard rules:
- 15×15 board with canonical TW/DW/TL/DL premium-square layout; centre is DW.
- 100-tile bag with the canonical letter distribution, 2 blanks at 0 pts.
- Racks of 7 tiles, refilled from the bag after each play.
- Premium squares apply only on the turn the tile first covers them.
- +50 bingo bonus when all 7 rack tiles are placed in one turn.
- Actions per turn: **play**, **exchange** (only if the bag has ≥ 7 tiles),
  or **pass**.
- Game ends when the bag is empty and a player empties their rack, or when
  every player passes twice in a row. Final scoring subtracts each player's
  remaining rack values from their score; the out-going player gains the sum
  of everybody else's leftover tiles.
- Server-side dictionary check: a play is rejected if any new word it forms is
  absent from the loaded dictionary. There is no challenge rule and no turn
  timer — this is an async format.

Bag order is derived from a seed stored on the game, so games are fully
replayable from the turn history for debugging.

## HTTP API

| Method | Path                           | Body / purpose                             |
|-------:|:-------------------------------|:-------------------------------------------|
| GET    | `/api/auth/config`             | available auth methods                     |
| POST   | `/api/auth/dev/login`          | `{userId, name}` — local dev login         |
| POST   | `/api/auth/dev/logout`         | clears dev cookie                          |
| POST   | `/api/auth/google/callback`    | `{credential}` — Google ID token exchange  |
| POST   | `/api/auth/google/logout`      | clears session cookie                      |
| GET    | `/api/users/me`                | current user                               |
| GET    | `/api/users/me/games`          | list my games                              |
| POST   | `/api/games`                   | `{numPlayers}` — create game (2–4 players) |
| POST   | `/api/games/join`              | `{inviteCode}` — join by code              |
| GET    | `/api/games/{id}`              | redacted game state for the caller         |
| POST   | `/api/games/{id}/plays`        | `{type: "play"\|"exchange"\|"pass", ...}`   |
| POST   | `/api/games/{id}/validate`     | `{placements}` — dry-run word validation   |
| GET    | `/api/push/vapid-key`          | VAPID public key for push subscription     |
| POST   | `/api/push/subscribe`          | store a Web Push subscription              |
| GET    | `/healthz`                     | health check                               |

Other players' racks and the bag contents are redacted server-side by
`viewFor()`; only a tile count is exposed.

## Features

All major planned features are implemented:

- **GCS-backed store** — optimistic concurrency via `x-goog-if-generation-match`;
  falls back to in-memory store for local dev.
- **Google Sign-In** — session cookies with HMAC-SHA256; dev login still
  available behind `-dev-login` flag.
- **Email allowlist** — `ALLOWLIST_EMAILS=a@x.com,b@y.com` (inline) or
  `ALLOWLIST_GCS=gs://bucket/file.txt` (refreshed every 5 min). Only gates
  game creation; anyone can sign in and join existing games. Unset = open.
- **Cloud Run deployment** — Terraform in `infra/`, 3-stage Dockerfile,
  scales to zero.
- **Web Push notifications** — VAPID-based; server pings the next player
  after each turn. Service worker registered on login.
- **UX polish** — tap-to-place (mobile-friendly), Escape recalls last tile,
  live score preview with server-side dictionary validation, off-turn tile
  staging, rack reorder (drag) and shuffle, end-game summary with winners.
- **`/healthz`** endpoint for Cloud Run health checks.

### Environment variables (production)

| Variable | Required | Source | Purpose |
|:---------|:---------|:-------|:--------|
| `BUCKET_NAME` | yes | env var | GCS bucket for game/user state |
| `GOOGLE_CLIENT_ID` | yes | env var | Google OAuth client ID |
| `SESSION_SECRET` | yes | Secret Manager | HMAC key for session cookies (hex) |
| `ALLOWLIST_EMAILS` | no | env var | Comma-separated allowed emails |
| `ALLOWLIST_GCS` | no | env var | `gs://bucket/object` path to allowlist file |
| `VAPID_PUBLIC_KEY` | no | env var | VAPID public key for Web Push |
| `VAPID_PRIVATE_KEY` | no | Secret Manager | VAPID private key for Web Push |
| `VAPID_CONTACT` | no | env var | Contact email for VAPID (e.g. `mailto:you@example.com`) |

## Known issues

- **Push notifications unverified** — subscriptions are now persisted to GCS but
  end-to-end delivery hasn't been confirmed yet

## Possible future work

- Game history/replay view
- Structured request logging
- Invite code garbage collection cron
- Daily billing-alert budget
