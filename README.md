# DeployFox

Single-owner deployment platform for one VPS: Go API and worker, Next.js 16 dashboard, PostgreSQL 16, Redis 7, isolated Docker agent and Caddy. Interface and beginner documentation are in Russian.

**This is an early implementation, not a verified production-ready release.** Read [verification and remaining work](docs/verification.md) before exposing it or trusting important data. No external VPS, real DNS/ACME or GitHub App account is configured in this checkout.

## Start locally

Docker Desktop (Linux containers) on Windows, or Docker Engine + Compose on Linux:

```bash
docker compose -f docker-compose.local.yml up -d --build
```

Open **http://localhost:3000**. Retrieve your generated one-time setup token privately:

```bash
docker compose -f docker-compose.local.yml exec api sh -c 'sed -n "s/^SETUP_TOKEN=//p" /run/deployfox/env'
```

Create the administrator with a password of at least 12 characters. Public registration does not exist. After initialization, the setup endpoint returns 404. Local mode uses an explicitly labelled mock provider: deployments transition and logs stream, but no user application is started. No shared or hardcoded administrator credentials are shipped.

Use `down` without `-v` to retain data. The secrets volume contains the local encryption key; deleting it makes stored secrets unreadable.

## Actual Docker operations

Production Compose uses a separate agent for Docker. API/frontend never mount the socket. Applications have resource/pid limits, no privileged mode or host mounts, dropped capabilities and no socket. Docker access still grants effective host privileges; this platform is only for your own trusted code.

For explicitly enabled real local deployments, see [development guide](docs/development.md). Default local mode never silently starts real application containers. Production refuses mock mode.

The Docker provider builds from Dockerfiles, falls back to pinned Railpack for supported source projects, starts a candidate, checks HTTP readiness, validates/reloads Caddy, then stops the old container. Failed candidates preserve the previous serving container. Rollback queues a previous successful local image with current environment variables. It does not roll back database schemas.

## Installation on Ubuntu

Start with the [Russian beginner guide](docs/00-start-here.md). It covers VPS selection, PowerShell SSH, keys and firewall, official Docker packages, copying this real checkout, domains, GitHub App, project examples, troubleshooting and recovery. There is deliberately no invented git-clone URL.

After prerequisites and DNS are configured, run from this checkout on Ubuntu 24.04:

```bash
sudo bash infrastructure/scripts/install.sh
```

The installer generates secrets, retains existing configuration and volumes, checks resources/ports, prepares a limited BuildKit builder, starts Compose and waits for healthchecks. It does not download or execute a remote installation shell script.

## Implementation map

- `backend/cmd/api`, `worker`, `agent`: separate process entrypoints in a modular monolith.
- `backend/internal`: auth, projects, deployments, Docker orchestration, GitHub App/webhooks, encrypted environment, databases, SSE logs, metrics, configuration and security.
- `backend/migrations`: embedded goose migrations, singleton administrator and partial unique index for active deployments.
- `frontend/src`: App Router, strict TypeScript, TanStack Query, Zustand, RHF/Zod, Montserrat through next/font/local, responsive dark/lime UI.
- `infrastructure`: Caddy configuration, installer, backup/restore and explicit integration-test scripts.
- `docs`: 21 numbered beginner chapters, development guide, OpenAPI notes and verification report.

OpenAPI: [specification](backend/internal/openapi/openapi.json), also `/api/v1/openapi.json` after login.

## Verification

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run build
```

Backend, real PostgreSQL/Redis integration, real Docker/Caddy/restore tests and isolated browser QA instructions: [docs/development.md](docs/development.md). Exact results and limitations: [docs/verification.md](docs/verification.md).

GitHub App secrets, domain ownership, firewall access, ACME issuance and offsite backup storage require your actual external infrastructure. Do not treat a local mock SUCCESS as a deployed public application.
