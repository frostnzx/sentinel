# MQTT topics and payloads

The API and simulator use `{MQTT_TOPIC_PREFIX}/` before every topic. Configure Mosquitto bridge directions explicitly when connecting a Pi. The simulator below is local-only.

| Local topic | Direction | Retained | Purpose |
|---|---|---:|---|
| `door/node/state` | Edge → cloud | No | Node telemetry and heartbeat |
| `door/node/alert` | Edge → cloud | No | PIR, vibration, tamper |
| `door/gateway/telemetry` | Edge → cloud | No | Gateway telemetry and heartbeat |
| `door/access/event` | Edge → cloud | No | Recognition outcome and metrics |
| `door/cmd/result` | Edge → cloud | No | Manual command acknowledgment |
| `door/faces/status` | Edge → cloud | No | Applied face-list version |
| `door/cmd/lock` | Cloud → edge | **No** | Expiring manual action command |
| `door/faces/sync` | Cloud → edge | Yes | Complete face-cache snapshot |

Telemetry includes a stable message ID and UTC `recorded_at`; the server adds `received_at`. Commands include a UUID, action, and UTC expiry. Devices must reject expired or previously executed IDs. Action commands are never retained.

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
