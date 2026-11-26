# Smart Door

The Go API connects to the public MQTT broker and stores combined five-second sensor snapshots and events in PostgreSQL/TimescaleDB. The Next.js dashboard calls the Go HTTP API; the browser does not connect to MQTT. Pi and ESP32 MQTT clients are owned by their teammates.

## Public broker mode

1. Copy `.env.example` to `.env` and replace `MQTT_TOPIC_PREFIX` with a unique value, for example `smartdoor-` plus the output of `openssl rand -hex 8`. Give the same prefix and broker URL to the Pi and ESP32 owners.
2. Set `MQTT_URL=ssl://test.mosquitto.org:8886` in `.env` (already the example default). No broker account or web-console setup is required for this anonymous test broker.
3. If the previous local stack is running, stop it with Ctrl+C and run `docker compose --profile sim down` so its simulator container is stopped. Then run `docker compose up --build`. Open the dashboard at http://localhost:3000 and Scalar API docs at http://localhost:8080/docs. The database persists in the Compose `db-data` volume. The local simulator is off in this mode.

The API subscribes to `<prefix>/sensor/{motion,door,distance,light,vibration}` plus `door/node/alert`, `door/access/event`, and `door/faces/status`. It publishes manual commands to `door/cmd/lock` without retention and complete retained face snapshots to `door/faces/sync`. It resubscribes after broker reconnects. [MQTT payload examples](docs/mqtt-topics.md) describe the current sensor contract.

The dashboard displays motion, door, vibration, distance, and light readings. Its Readings page shows combined five-second logs with date filters and older-history loading. The door uses `raw: 0` for closed and `raw: 1` for open while its reed sensor is uncalibrated; other invalid sensor values show as unknown. Manual lock/unlock requests can be sent without recent sensor readings when MQTT is connected; they show “command sent” after the broker confirms publication. The API does not track device execution. The API also stores access events and exposes them at `GET /api/v1/devices/door-01/events`. `POST /api/v1/faces/sync` accepts a complete versioned list of embeddings and `GET /api/v1/faces/sync` shows the version acknowledged by the Pi. Enrollment photo processing and its dashboard page are later work.

## Local simulation

For fake readings and simulated command handling, run `MQTT_URL=tcp://mqtt:1883 docker compose --profile sim up --build`. This starts local Mosquitto and the simulator. Use plain `docker compose up --build` with the public `MQTT_URL` to return to real-device mode.

To publish one development reading for every sensor to the public broker, set `MQTT_TOPIC_PREFIX` in `.env` and run `make publish-dev-sensors`. The target publishes motion detected, door open, vibration detected, 85 mm distance, and 32.5 lux. It creates fresh boot IDs on every run so the API does not discard repeated development messages as duplicates. Override the broker with `MQTT_HOST` and `MQTT_PORT` when needed.

## Contracts and limitations

[OpenAPI](contracts/openapi.json) is the shared HTTP contract; run `make generate` after changing it. The public test broker permits others to publish and read raw topics, including lock commands and face embeddings. Its topic prefix avoids accidental collisions but does not control access. Public-broker automatic unlocking stops when the internet or broker is unavailable.

## Visitor photos and analysis

The Pi can upload a JPEG or PNG up to 5 MiB with the same event UUID as the MQTT access event. Both arrival orders are supported and the first photo for an event is kept for 30 days.

```sh
curl -F 'event_id=EVENT_UUID' -F 'image=@visitor.jpg' http://localhost:8080/api/v1/snapshots
```

Set `VISITOR_ANALYSIS_MODE=stub` for a labeled simulated result, or `VISITOR_ANALYSIS_MODE=gemini` and `GEMINI_API_KEY` to use Gemini. `GEMINI_MODEL` defaults to `gemini-3.1-flash-lite`. The API analyzes photos attached to `lockout`, `spoof`, or `loiter` events, at most once every 30 seconds. Additional eligible events in that window are skipped. Analysis is off by default. The API key belongs in the server environment, never the frontend. Confirm the event kinds and photo contract with the Pi team before hardware integration.

## Local trusted-face enrollment

Compose mounts the bundled YuNet and SFace files from [`models/`](models/) read-only. Set `FACE_MODELS_DIR` to use a different directory. Run `MQTT_URL=tcp://mqtt:1883 docker compose --profile sim --profile enrollment up --build` using a unique topic prefix. The Trusted Faces page accepts 3–5 JPEG or PNG photos per person. The face service detects one face, aligns it, and returns SFace embeddings. Uploaded enrollment photos are discarded after processing. The simulator acknowledges gallery versions on the local broker.

`FACE_MODEL_VERSION` defaults to `local-sface-2021dec` for the local demo. Coordinate model version, model hashes, and preprocessing with the Pi team before real-device integration. YuNet SHA-256 `8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4`. SFace SHA-256 `0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79`. Public MQTT sends complete retained embeddings and is not isolated.
