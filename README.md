# not-scrabble

Self-hostable, turn-based, async Scrabble. Single Go binary that serves the JSON
API and an embedded React/TypeScript frontend. Designed to later run on Cloud
Run + GCS but currently uses an in-memory store so you can play end-to-end on
your laptop with no cloud setup.

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
takes any `userId` + display name and sets a cookie — no Google Sign-In wired
up yet. Create a game in one window, copy the invite code into the other,
join, then have the creator click **Start game**.

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

## Dictionary

On startup the server tries to load `data/enable.txt` and falls back to a
tiny built-in list (~78 common words) if that's missing. The fallback is
enough to sanity-check a play flow but too small for real games.

Fetch ENABLE (public domain, 172,819 words) from Norvig's mirror:

```sh
curl -o data/enable.txt https://norvig.com/ngrams/enable1.txt
```

Point `-dict` at any other newline-separated word file if you prefer:

```sh
go run ./cmd/server -dict /path/to/your/wordlist.txt
# or the gzipped form
go run ./cmd/server -dict /path/to/your/wordlist.txt.gz
```

Copyrighted tournament lists (TWL, SOWPODS/Collins, NWL) are not included;
keep any local copy under `data/` (already `.gitignore`'d) and do not commit.

## Project layout

```
cmd/server/         # main.go — HTTP server entrypoint
internal/game/      # pure Scrabble engine (board, bag, rack, scoring, validation)
internal/dict/      # newline-separated word-list loader
internal/store/     # game/user persistence (in-memory today, GCS later)
internal/httpapi/   # HTTP handlers, request/response types, auth middleware
web/                # React + TypeScript + Vite frontend
webdist/            # Go package that //go:embeds the built frontend
```

The Go binary embeds `webdist/dist/` at build time via `//go:embed`. A
`placeholder.txt` keeps the embed compiling on a fresh clone before you've run
`npm run build`. `vite.config.ts` uses `emptyOutDir: false` so the placeholder
survives rebuilds.

## Gameplay rules

Standard English Scrabble:
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
| POST   | `/api/auth/dev/login`          | `{userId, name}` — local dev login         |
| POST   | `/api/auth/dev/logout`         | clears dev cookie                          |
| GET    | `/api/users/me`                | current user                               |
| GET    | `/api/users/me/games`          | list my games                              |
| POST   | `/api/games`                   | create a new game (creator is player 1)    |
| POST   | `/api/games/join`              | `{inviteCode}` — join by code              |
| GET    | `/api/games/{id}`              | redacted game state for the caller         |
| POST   | `/api/games/{id}/start`        | creator starts when 2–4 players have joined |
| POST   | `/api/games/{id}/plays`        | `{type: "play"\|"exchange"\|"pass", ...}`   |

Other players' racks and the bag contents are redacted server-side by
`viewFor()`; only a tile count is exposed.

## Next steps

Rough priority order; pick any block and go:

1. **GCS-backed store** (`internal/store/gcs.go`). Drop-in for `store.Store`
   using `cloud.google.com/go/storage` with `x-goog-if-generation-match`
   preconditions on `UpdateGame`. Add an integration test against
   [fake-gcs-server](https://github.com/fsouza/fake-gcs-server), including a
   simulated racing write that must be rejected with 412.
2. **Google Sign-In** (`internal/auth/google.go`). Swap `DevAuth` for an
   `Authenticator` that verifies ID tokens with
   `google.golang.org/api/idtoken`, extracts `sub`/`email`/`name`, and sets a
   short-lived session cookie. Wire the Google Identity Services script into
   the frontend login view. Keep `DevAuth` behind the `-dev-login` flag for
   local hacking.
3. **Optional email allowlist** (`internal/auth/allowlist.go`). Gate access
   to an invite-only set of Google accounts. Config resolution order:
   - unset → open (anyone with a Google account can sign in),
   - `ALLOWLIST_EMAILS=alice@x.com,bob@y.com` → parse inline,
   - `ALLOWLIST_GCS=gs://bucket/allowlist.txt` → fetch + cache with a
     ~5-minute refresh, so you can edit the object in the console without
     redeploying.
   Check happens after ID-token verification; reject with 403 before any
   user record is created. Case-insensitive match on the verified `email`
   claim. Expose a tiny `GET /api/auth/status` so the frontend can render
   "you're not on the guest list" cleanly instead of a bare 403.
4. **Cloud Run deploy.** 3-stage `Dockerfile` (node build → go build →
   distroless/static). `gcloud run deploy --source=.`. Bucket + dedicated
   service account with `roles/storage.objectAdmin` scoped to the one bucket.
   Configure via env: `BUCKET_NAME`, `GOOGLE_CLIENT_ID`, optional
   `ALLOWLIST_EMAILS` or `ALLOWLIST_GCS`. Keep `min-instances=0` so the free
   tier actually stays free. Document the one-off bucket-create and IAM
   setup in this README.
5. **Web Push for turn alerts.** Generate VAPID keys; `POST /api/push/subscribe`
   stores the subscription under the user's Google `sub` so laptop + phone
   both get pinged. Server fires a push after each turn commit. Fall back to
   polling (already wired) when a browser has no subscription.
6. **UX polish.** Tap-to-place as an alternative to drag (for phones that
   don't handle `TouchSensor` well); game-history view; "recall last tile"
   keyboard shortcut; exchange confirmation modal; end-game summary screen
   with per-player leftover tiles.
7. **Operational niceties.** Structured request logging, a `/healthz` for
   Cloud Run, a cron job that garbage-collects `invites/*.json` older than
   N days, a daily billing-alert budget.

The original plan (architecture/cost reasoning) lives at
`/Users/alan/.claude/plans/snappy-wiggling-wand.md`.
