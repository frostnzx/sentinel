# MQTT topics and payloads

The Pi, ESP32, and API use the same unique `{MQTT_TOPIC_PREFIX}/` on `test.mosquitto.org`. The browser uses HTTP. Local Mosquitto and the simulator run only with the `sim` Compose profile.

| Local topic | Direction | Retained | Purpose |
|---|---|---:|---|
| `door/node/state` | Edge → cloud | No | Node telemetry and heartbeat |
| `door/node/alert` | Edge → cloud | No | PIR, vibration, tamper |
| `door/gateway/telemetry` | Edge → cloud | No | Gateway telemetry and heartbeat |
| `door/access/event` | Edge → cloud | No | Recognition outcome and metrics |
| `door/cmd/result` | Edge → cloud | No | Manual command acknowledgment |
| `door/faces/status` | Edge → cloud | No | Applied face-list version |
| `door/cmd/lock` | API/Pi → ESP32 | **No** | Expiring manual or automatic action command |
| `door/faces/sync` | Cloud → edge | Yes | Complete face-cache snapshot |

Telemetry includes a stable message ID and UTC `recorded_at`; the server adds `received_at`. Commands include a UUID, action, and UTC expiry. Devices must reject expired or previously executed IDs. Action commands are never retained.

The Pi sends `origin: "auto"` on automatic unlock; the API sends `origin: "manual"` on manual commands. Both use the same result topic. Face sync is a complete retained JSON object with `version`, `model_version`, and `people`; each person has an ID, name, and 2–5 embeddings of 128 numbers. An empty `people` list clears the cache. The Pi acknowledges with `{"version":1,"status":"applied"}` or `{"version":1,"status":"error","error":"reason"}`.

Example node state:

```json
{"id":"8ea2e51e-ef9b-4e6b-9682-b35722c2cf27","source":"node","door_open":false,"lock_state":"locked","presence":false,"temperature_c":24.2,"recorded_at":"2026-01-02T03:04:05Z"}
```

Example gateway telemetry:

```json
{"id":"48600d65-7aae-42e2-83d3-d2fc540b1cbc","source":"gateway","distance_mm":620,"light_lux":88.5,"recorded_at":"2026-01-02T03:04:05Z"}
```

Example result:

```json
{"id":"command-uuid","status":"succeeded","message":"Unlock command executed","recorded_at":"2026-01-02T03:04:06Z"}
```
