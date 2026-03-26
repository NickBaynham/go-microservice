# Web UI (React + Vite)

Minimal SPA: **register**, **log in**, **forgot / reset password**, and **account** (`GET /me`). The API base URL is set at **build time** via `VITE_API_BASE_URL`. End-user steps: **[../docs/END_USER_GUIDE.md](../docs/END_USER_GUIDE.md)**.

## Local development

1. Run the API with HTTP (same JWT secret you use for login), for example:

   ```bash
   LISTEN_HTTP=true PORT=8080 JWT_SECRET=test-jwt-secret-do-not-use-in-prod make run
   ```

2. Copy env and install:

   ```bash
   cp .env.example .env.local
   # edit VITE_API_BASE_URL if needed
   npm install
   npm run dev
   ```

3. Open the URL Vite prints (default `http://localhost:5173`).  
   Ensure the API has `CORS_ALLOWED_ORIGINS` including your Vite origin (defaults in **development** include `http://localhost:5173`).

## Production / AWS

1. Build the SPA with the **public API URL**:

   ```bash
   VITE_API_BASE_URL=https://api.yourdomain.com npm run build
   ```

2. Deploy `dist/` to static hosting (S3/CloudFront, etc.).

3. On the **ECS task** (or wherever the API runs), set:

   `CORS_ALLOWED_ORIGINS=https://app.yourdomain.com`

   (your real SPA origin, comma-separated if multiple).

## End-to-end tests (Playwright)

| Mode | Description |
|------|-------------|
| **Local stack + host Playwright** | Start API + web with Compose, run tests on your machine. |
| **All in Docker** | Compose override runs Playwright in a container with the correct API hostname in the bundle. |

### Host Playwright (default Compose)

From the **repository root**:

```bash
docker compose -f deployments/docker-compose.e2e.yml up -d --build
# wait until http://127.0.0.1:18080/health and http://127.0.0.1:14173/ respond
cd frontend
npm ci
npx playwright install chromium
E2E_BASE_URL=http://127.0.0.1:14173 E2E_SKIP_WEB_SERVER=1 npm run test:e2e
docker compose -f deployments/docker-compose.e2e.yml down -v
```

The SPA image is built with `VITE_API_BASE_URL=http://127.0.0.1:18080` so the **browser on your host** reaches the published API port.

### Playwright inside Docker

Rebuild the SPA so XHR uses the internal service name `api-e2e`:

```bash
docker compose -f deployments/docker-compose.e2e.yml -f deployments/docker-compose.e2e.runner.yml up --build --abort-on-container-exit playwright-runner
```

### Self-signed HTTPS API

If the API is only available over HTTPS with an untrusted cert:

```bash
E2E_IGNORE_HTTPS_ERRORS=1 npm run test:e2e
```

### CI

Pull requests run **Frontend E2E** in GitHub Actions (Compose + Playwright on the runner).

### Environment variables

| Variable | Purpose |
|----------|---------|
| `E2E_BASE_URL` | Origin of the SPA for Playwright (default `http://127.0.0.1:4173`). |
| `E2E_SKIP_WEB_SERVER` | `1` / `true` — do not start `vite preview` (Compose already serves the app). |
| `E2E_API_BASE_URL` | Used when Playwright starts its own `webServer` to **build** the app (default `http://127.0.0.1:8080`). |
| `E2E_IGNORE_HTTPS_ERRORS` | `1` / `true` — ignore TLS errors in the browser. |
