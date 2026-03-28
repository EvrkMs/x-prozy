# x-prozy — AI Agent Instructions

## What This Project Is

x-prozy is a **new Xray proxy control panel** built from scratch in Go. It borrows UI/UX patterns from `3x-ui` (in `ref/3x-ui/`) but has a completely different architecture. The `ref/` directory is **read-only reference material** — never modify files there.

## Architecture

### Panel Backend (`panel/panel-backend/`)

Minimal Go 1.26 app with **no framework** — stdlib `net/http` + `html/template`. Key layers:

| Layer | Path | Role |
|---|---|---|
| Entry | `cmd/panel/main.go` | Bootstrap: config → logger → `web.NewApp()` → `http.ListenAndServe` |
| Web | `internal/web/app.go` | All routes, handlers, page data. **Single file is the router** |
| Auth | `internal/auth/` | bcrypt login, session tokens, user CRUD. Exposes `SessionValidator` interface for middleware |
| Settings | `internal/settings/` | Key-value store in SQLite for runtime config (session duration, secret path) |
| Middleware | `internal/middleware/` | `RequireAuth` (cookie-based), `Recover`, `RequestID`, `Logger`. Uses `Chain()` pattern from `stack.go` |
| Render | `internal/render/` | Template engine with dev hot-reload. Pages define named templates (`dashboard_page`, `login_page`) |
| Config | `internal/config/` | ENV-only infra config (`DB_PATH`, `PANEL_ADDR`, `PANEL_PORT`). Business settings live in DB |

### Frontend (server-rendered)

No JS framework. Pure HTML/CSS/JS in `web/`:
- `web/templates/layouts/base.html` — shared `<head>` (`head_assets` template)
- `web/templates/pages/{login,dashboard}.html` — full page templates
- `web/static/css/auth.css` — CSS variables (`:root`) + login page styles
- `web/static/css/dashboard.css` — dashboard layout, sidebar, sli-list pattern, light theme
- `web/static/js/dashboard.js` — tab switching, API form handler (fetch+JSON), toast, theme toggle

### CSS Architecture

Global variables are in `auth.css :root`. Dashboard and login both import `auth.css`.
Light theme: `html.light` class overrides all variables. Persisted via `localStorage('prozy-theme')`.
Applied before paint via inline `<script>` in `base.html`.

### Setting List Item Pattern (`sli`)

Dashboard settings use a 3x-ui–inspired row layout (class prefix `sli`):
```html
<div class="sli-list">
  <div class="sli-list__head">...</div>
  <form data-api="/api/settings/..." class="sli">
    <div class="sli__meta"><div class="sli__title">...</div><div class="sli__desc">...</div></div>
    <div class="sli__control"><input class="sli__input" ...><button class="sli__save">Save</button></div>
  </form>
</div>
```
Grid is `minmax(180px, 0.45fr) 1fr` — meta shrinks, control stretches.

## Build & Deploy

```bash
# Local dev (from x-prozy/)
docker compose up -d --build    # builds Go binary in multi-stage Dockerfile, starts container
docker compose logs -f panel    # tail panel logs

# The panel is at http://<host>:8080/<secret-path>
# Secret path is stored in SQLite, currently /tnlp

# Direct DB access for debugging
docker exec x-prozy-panel sqlite3 /app/data/panel.db ".tables"
```

**No hot-reload**: HTML/CSS/JS are baked into Docker image. Every change requires `docker compose up -d --build`.

## Key Conventions

1. **Routes live in `app.go`** — `Routes()` method registers everything. Protected routes use `protected := middleware.RequireAuth(...)`. API routes return JSON, page routes render templates.

2. **API pattern** — All settings forms use `data-api="/api/settings/..."` attribute. JS intercepts submit, sends JSON via `fetch`, handles `redirect` / `redirect_login` in response.

3. **Secret path** — Panel hides behind a URL prefix (`/tnlp`). The `secretPathHandler` middleware strips the prefix. Static files (`/static/`) and `/healthz` bypass it.

4. **Auth flow** — Cookie `prozy_session` → `middleware.RequireAuth` → `auth.SessionValidator.ValidateToken()` → `middleware.UserFromContext(ctx)`. Password changes invalidate session and return `"redirect_login": true`.

5. **Settings vs Config** — ENV (`config.go`) = infra (address, port, DB path). DB (`settings/`) = business (session duration, secret path). Never mix them.

6. **Template naming** — Pages define `{{ define "dashboard_page" }}`. The render engine calls them by name, not filename.

## Project Vision (from AGENTS.md)

x-prozy will grow into a multi-node Xray management system:
- **Client-first model**: clients are primary entities, not embedded in inbound JSON
- **Node-aware**: panel = control plane, node = data plane (separate `node/` module planned)
- **Runtime projection**: Xray config built from normalized DB entities, not ad-hoc JSON

When adding features, treat `3x-ui` as a **UX reference** (protocol forms, modals, generation helpers) — not as architecture to copy.
