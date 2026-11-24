# Repository Guidelines

## Project Structure & Architecture

`smart-door-plan.md` remains the source of truth for product scope and implementation phases, with the public-broker integration decision documented at its top. The application lives in `backend/` (Go API, MQTT simulator, and TimescaleDB migrations), `frontend/` (Next.js dashboard), `contracts/` (OpenAPI JSON), `infra/` (simulation Mosquitto configuration), and the root `docker-compose.yml`. Pi and ESP32 code is teammate-owned and is not in this repository.

The plan calls for a Next.js/TypeScript dashboard, a Go/Chi API, PostgreSQL with TimescaleDB, and a teammate-owned Python AI service. A Raspberry Pi and ESP32 handle local recognition and door control; the cloud side communicates through MQTT and HTTP. Keep automatic unlocking operational independently of cloud services.

## Development, Build & Testing

Run the real-device stack with `docker compose up --build` after setting a unique `MQTT_TOPIC_PREFIX` in `.env`. Run the local simulator stack with `MQTT_URL=tcp://mqtt:1883 docker compose --profile sim up --build`. The API applies migrations on startup. Build Go with `cd backend && go build ./cmd/api ./cmd/simulator`; build the dashboard with `cd frontend && npm ci && npm run build`. Run `make generate` after changing `contracts/openapi.json` to refresh Go and TypeScript bindings.

Follow the plan’s sequence: define contracts, establish the local foundation, then implement simulation and backend flows before hardware integration and deployment. The current MQTT milestone connects the API, Pi, and ESP32 directly to the public broker. Enrollment photo processing, snapshots, event analysis, the remaining dashboard pages, full hardware verification, and GCP deployment are still pending.

## Coding & Contracts

Use the planned `/api/v1` HTTP prefix and generate Go and TypeScript types from the same OpenAPI specification. Keep MQTT topic prefixes configurable, use unique client IDs, and include IDs for duplicate handling. Never retain action commands. Use UTC timestamps and preserve separate device and server receipt times.

Follow each language’s standard formatting conventions and add formatter and linter commands when tooling exists. Name new files and tests consistently with the conventions established in their respective modules.

## Testing Guidelines

Add focused automated checks for API behavior, persistence, MQTT handling, contract consistency, and builds. Cover duplicate and expired commands, enrollment and synchronization failures, and snapshot/event arrival order. Report simulator results separately from real-hardware verification. No testing framework or coverage threshold has been established yet.

## Commits, Reviews & Configuration

Git history is unavailable here, so no existing commit convention can be inferred. Use concise, descriptive commit subjects. Pull requests should explain the change, list verification performed, link relevant issues when available, and include screenshots for dashboard changes.

Keep credentials out of version control and supply runtime settings through configuration. The planned public MQTT broker does not isolate project traffic; a unique topic prefix only prevents accidental collisions. Restrict deployment access as described in the plan.
