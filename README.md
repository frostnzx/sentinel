# Smart Door

Local development foundation for the smart-door project. The first milestone is a simulated reading shown in the dashboard and a manual command with a simulated acknowledgment.

## Local setup

Requirements: Docker Compose, Go 1.24+, and Node.js 20+.

```sh
# Optional: copy .env.example to .env to override defaults.
docker compose up --build
```

Open the dashboard at http://localhost:3000 and the API documentation at http://localhost:8080/docs. The simulator publishes readings through Mosquitto every five seconds. API and MQTT settings are in `.env.example`.

The public broker is intentionally unset until the team selects one. The local stack does not connect to a public broker or real hardware. It has no application authentication; keep it on a trusted network.

## Implemented first slice

- Shared OpenAPI contract and MQTT topic map.
- Go/Chi API, TimescaleDB schema, local Mosquitto, and Docker Compose.
- MQTT simulator and dashboard view for live state, readings, and manual lock/unlock acknowledgment.
- Generation commands for Go and TypeScript types from the shared OpenAPI JSON (`make generate`).

The remaining plan phases are real enrollment and snapshot/AI flows, the four complete dashboard pages, hardware integration, and GCP/Terraform deployment. See [the full plan](docs/smart-door-plan.md).
