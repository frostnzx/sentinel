# Sentinel Raspberry Pi integration handoff

Sentinel enrolls trusted people through the web dashboard. The Go API calls the internal Python `face-ai` service to turn enrollment photos into embeddings and publishes the resulting gallery over MQTT. The Pi keeps a local copy of that gallery and uses its own camera and models to detect and match faces at the door.

## Set up the two sides

- **Backend owner:** Run the Sentinel [Compose stack](../docker-compose.yml) with the `enrollment` profile enabled. Compose mounts the bundled [model files and licenses](../models/) read-only into `face-ai`. For real device integration, leave the `sim` profile off so that the simulator cannot acknowledge gallery updates on the Pi’s behalf.
- **Pi developer:** Pull the Sentinel revision shared by the backend owner and copy the two ONNX files from `models/` into your Pi app’s model directory. If the app runs in Docker, mount the files into that container. The Pi runs local recognition and talks to the backend over MQTT and HTTP.
- **Shared settings:** Get the private MQTT broker URL and credentials, the shared topic prefix, and the reachable backend base URL from the backend owner. Use a unique MQTT client ID for the Pi. Use the private broker for real galleries, since retained messages contain names and face embeddings. Examples below use `<prefix>` for the shared topic prefix and `SENTINEL_BASE_URL` for the backend URL without `/api/v1`.

## Use the same models and alignment

| Purpose | File | SHA-256 |
| --- | --- | --- |
| YuNet face detection | `face_detection_yunet_2023mar.onnx` | `8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4` |
| SFace embedding | `face_recognition_sface_2021dec.onnx` | `0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79` |

Match the [enrollment code](../ai/app.py) when producing a live embedding:

1. Use a BGR camera frame. Set YuNet’s input size to the frame’s width and height, then run detection.
2. Pass the frame and the selected YuNet detection row to OpenCV `FaceRecognizerSF.alignCrop`.
3. Call `FaceRecognizerSF.feature` on the aligned image and flatten the result into 128 numbers. Compare it with the gallery embeddings locally on the Pi.

The expected `model_version` for the first integration is `local-sface-2021dec`, the backend default. If the project changes that value, configure both sides together.

## Receive and acknowledge the gallery

Subscribe to `<prefix>/door/faces/sync`. The backend publishes the complete gallery as a retained MQTT message with `version`, `model_version`, and `people`. Each person has `id`, `name`, and `embeddings` with 2–5 vectors of 128 numbers each.

Check `model_version` before replacing the local gallery. Keep the last usable gallery if the incoming version of the model is incompatible. Apply valid snapshots as full replacements, including an empty `people` list, which clears the gallery.

Publish one of these responses to `<prefix>/door/faces/status` without retention. Use the gallery `version` from the message you received, and report `applied` only after updating the local gallery.

```json
{"version":1,"status":"applied"}
```

```json
{"version":1,"status":"error","error":"Unsupported model version"}
```

## Send access events and visitor photos

Publish the recognition outcome to `<prefix>/door/access/event` without retention. Generate a UUID for `id` and use a UTC timestamp in `recorded_at`.

```json
{"id":"0baf0e32-239a-4f98-b345-9a0fd4b0cd1f","kind":"lockout","recorded_at":"2026-11-26T08:15:00Z"}
```

If you have a visitor photo for that event, upload it as multipart form data. Use the same UUID as `event_id`. The `image` field accepts a JPEG or PNG up to 5 MiB. The event and photo can arrive in either order. The backend keeps the first photo per event.

```sh
curl --fail-with-body \
  -F 'event_id=0baf0e32-239a-4f98-b345-9a0fd4b0cd1f' \
  -F 'image=@visitor.jpg' \
  "${SENTINEL_BASE_URL}/api/v1/snapshots"
```

If Gemini analysis is enabled on the backend, photos attached to `lockout`, `spoof`, or `loiter` events are eligible for analysis. The Pi’s local recognition uses the gallery regardless of that setting. See the [MQTT topic reference](mqtt-topics.md) and [HTTP API contract](../contracts/openapi.json) for the wider device integration.

## Check the integration when hardware is available

Enrollment and gallery acknowledgment have been verified locally with the bundled models and a simulated Pi. The Pi team can verify the hardware flow when their device and the shared backend are available:

- [ ] Enroll one test person in **Trusted Faces** and confirm the real Pi receives and acknowledges that gallery version.
- [ ] Scan that person at the Pi and confirm a local match against the downloaded gallery.
- [ ] In a test environment, send a snapshot with a mismatched `model_version`. Confirm the Pi reports an error and keeps the previously applied gallery.
- [ ] Publish an access event and upload a photo with the same UUID. Confirm they appear together in **Access History**.
