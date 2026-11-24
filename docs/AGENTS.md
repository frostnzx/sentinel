# Repository Guidelines

## Project Structure & Architecture

The repository currently contains `smart-door-plan.md`, the source of truth for the proposed smart-door system, its integration contracts, and implementation phases. Application code, tests, assets, and deployment configuration have not been added yet. Update this guide as those directories and tools are introduced.

The plan calls for a Next.js/TypeScript dashboard, a Go/Chi API, PostgreSQL with TimescaleDB, and a teammate-owned Python AI service. A Raspberry Pi and ESP32 handle local recognition and door control; the cloud side communicates through MQTT and HTTP. Keep automatic unlocking operational independently of cloud services.

## Development, Build & Testing

No build, run, lint, or test commands are configured in this repository yet. Introduce and document reproducible commands alongside the first implementation, including dependency setup, database migrations, local services, and the simulator. Do not present proposed commands as already working.

Follow the plan’s sequence: define OpenAPI and MQTT contracts, establish the local foundation, then implement simulation and backend flows before hardware integration and deployment. The first milestone is a simulated reading displayed in the dashboard and a manual command with a simulated acknowledgment.

## Coding & Contracts

Use the planned `/api/v1` HTTP prefix and generate Go and TypeScript types from the same OpenAPI specification. Keep MQTT topic prefixes configurable, use unique client IDs, and include IDs for duplicate handling. Never retain action commands. Use UTC timestamps and preserve separate device and server receipt times.

Follow each language’s standard formatting conventions and add formatter and linter commands when tooling exists. Name new files and tests consistently with the conventions established in their respective modules.

## Testing Guidelines

Add focused automated checks for API behavior, persistence, MQTT handling, contract consistency, and builds. Cover duplicate and expired commands, enrollment and synchronization failures, and snapshot/event arrival order. Report simulator results separately from real-hardware verification. No testing framework or coverage threshold has been established yet.

## Commits, Reviews & Configuration

Git history is unavailable here, so no existing commit convention can be inferred. Use concise, descriptive commit subjects. Pull requests should explain the change, list verification performed, link relevant issues when available, and include screenshots for dashboard changes.

Keep credentials out of version control and supply runtime settings through configuration. The planned public MQTT broker does not isolate project traffic; a unique topic prefix only prevents accidental collisions. Restrict deployment access as described in the plan.
