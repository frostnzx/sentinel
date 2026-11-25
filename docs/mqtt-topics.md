# MQTT topics and payloads

The Pi, ESP32, and API use the same unique `{MQTT_TOPIC_PREFIX}/` on `test.mosquitto.org`. The browser uses HTTP. Local Mosquitto and the simulator run only with the `sim` Compose profile.

| Local topic | Direction | Retained | Purpose |
|---|---|---:|---|
| `sensor/motion` | ESP32 → cloud | No | Motion state |
| `sensor/door` | ESP32 → cloud | No | Calibrated reed state |
| `sensor/vibration` | ESP32 → cloud | No | Vibration state |
| `sensor/distance` | Pi → cloud | No | Distance in millimetres |
| `sensor/light` | Pi → cloud | No | Illuminance in lux |
| `door/node/alert` | Edge → cloud | No | PIR, vibration, tamper |
| `door/access/event` | Edge → cloud | No | Recognition outcome and metrics |
| `door/cmd/result` | Edge → broker | No | Optional device execution result; the API does not subscribe |
| `door/faces/status` | Edge → cloud | No | Applied face-list version |
| `door/cmd/lock` | API/Pi → ESP32 | **No** | Expiring manual or automatic action command |
| `door/faces/sync` | Cloud → edge | Yes | Complete face-cache snapshot |

Sensor messages carry `device_id`, `boot_id`, `seq`, `simulated`, `valid`, and `value`. The ESP32 sends `uptime_ms` without a wall-clock timestamp, so the API uses receipt time for its sensor timestamp. The Pi sends UTC `ts` as its sensor timestamp; the API also tracks server receipt time. A valid door `value: true` means closed, and `false` means open. While the reed sensor is uncalibrated, the door status uses `raw: 0` for closed and `raw: 1` for open and still exposes the calibration error. Other invalid readings remain unknown. The API ignores duplicate or older sensor sequences and writes one combined readings-history row at each UTC-aligned five-second boundary while any sensor is fresh. Values older than 30 seconds are null; the `samples` object records each included sensor's validity, error, device identity, sequence, and timestamps. The row's `recorded_at` is the interval boundary and `received_at` is the write time. Existing per-message history remains queryable until the 30-day retention policy removes it. Commands include a UUID, action, and UTC expiry. Devices must reject expired or previously executed IDs. Action commands are never retained.

The Pi sends `origin: "auto"` on automatic unlock; the API sends `origin: "manual"` on manual commands. The API responds to a manual HTTP request once the broker confirms publication; it does not wait for `door/cmd/result` or track command outcomes. Devices may still publish results for other consumers. Face sync is a complete retained JSON object with `version`, `model_version`, and `people`; each person has an ID, name, and 2–5 embeddings of 128 numbers. An empty `people` list clears the cache. The Pi acknowledges with `{"version":1,"status":"applied"}` or `{"version":1,"status":"error","error":"reason"}`.

Example ESP32 motion, invalid door, and vibration readings (in topic order):

```json
{"device_id":"esp32-94C3C14EB580","boot_id":"DD088E1F","seq":507,"uptime_ms":169823,"simulated":false,"value":false,"valid":true,"raw":0}
{"device_id":"esp32-94C3C14EB580","boot_id":"DD088E1F","seq":508,"uptime_ms":170820,"simulated":false,"value":null,"valid":false,"raw":1,"meaning":"door_open","error":"reed_not_calibrated"}
{"device_id":"esp32-94C3C14EB580","boot_id":"DD088E1F","seq":509,"uptime_ms":170822,"simulated":false,"value":false,"valid":true,"pulses":0,"window_ms":1001}
```

Example Pi distance and light readings:

```json
{"device_id":"pi5-gateway","boot_id":"0c0cbfaa","seq":165,"ts":"2026-11-24T08:19:26.160+00:00","uptime_ms":82049,"simulated":false,"value":68.0,"valid":true,"unit":"mm"}
{"device_id":"pi5-gateway","boot_id":"0c0cbfaa","seq":166,"ts":"2026-11-24T08:19:26.161+00:00","uptime_ms":82049,"simulated":false,"value":17.5,"valid":true,"unit":"lux"}
```

Example result:

```json
{"id":"command-uuid","status":"succeeded","message":"Unlock command executed","recorded_at":"2026-01-02T03:04:06Z"}
```
