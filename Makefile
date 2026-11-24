-include .env

MQTT_HOST ?= test.mosquitto.org
MQTT_PORT ?= 1883

.PHONY: generate generate-go generate-ts publish-dev-sensors

generate: generate-go generate-ts

generate-go:
	cd backend && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 --config oapi-codegen.yaml ../contracts/openapi.json

generate-ts:
	cd frontend && npm run generate:api

publish-dev-sensors:
	@test -n "$(MQTT_TOPIC_PREFIX)" || (echo "Set MQTT_TOPIC_PREFIX in .env" >&2; exit 1)
	@command -v mosquitto_pub >/dev/null || (echo "mosquitto_pub is required" >&2; exit 1)
	@mosquitto_pub -h "$(MQTT_HOST)" -p "$(MQTT_PORT)" -q 0 -t "$(MQTT_TOPIC_PREFIX)/sensor/motion" -m "{\"device_id\":\"esp32-94C3C14EB580\",\"boot_id\":\"dev-motion-$$(date +%s%N)\",\"seq\":1,\"uptime_ms\":1000,\"simulated\":true,\"value\":true,\"valid\":true,\"raw\":1}"
	@mosquitto_pub -h "$(MQTT_HOST)" -p "$(MQTT_PORT)" -q 0 -t "$(MQTT_TOPIC_PREFIX)/sensor/door" -m "{\"device_id\":\"esp32-94C3C14EB580\",\"boot_id\":\"dev-door-$$(date +%s%N)\",\"seq\":1,\"uptime_ms\":1000,\"simulated\":true,\"value\":null,\"valid\":false,\"raw\":1,\"meaning\":\"door_open\",\"error\":\"reed_not_calibrated\"}"
	@mosquitto_pub -h "$(MQTT_HOST)" -p "$(MQTT_PORT)" -q 0 -t "$(MQTT_TOPIC_PREFIX)/sensor/vibration" -m "{\"device_id\":\"esp32-94C3C14EB580\",\"boot_id\":\"dev-vibration-$$(date +%s%N)\",\"seq\":1,\"uptime_ms\":1000,\"simulated\":true,\"value\":true,\"valid\":true,\"pulses\":1,\"window_ms\":1000}"
	@mosquitto_pub -h "$(MQTT_HOST)" -p "$(MQTT_PORT)" -q 0 -t "$(MQTT_TOPIC_PREFIX)/sensor/distance" -m "{\"device_id\":\"pi5-gateway\",\"boot_id\":\"dev-distance-$$(date +%s%N)\",\"seq\":1,\"ts\":\"$$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)\",\"uptime_ms\":1000,\"simulated\":true,\"value\":85.0,\"valid\":true,\"unit\":\"mm\"}"
	@mosquitto_pub -h "$(MQTT_HOST)" -p "$(MQTT_PORT)" -q 0 -t "$(MQTT_TOPIC_PREFIX)/sensor/light" -m "{\"device_id\":\"pi5-gateway\",\"boot_id\":\"dev-light-$$(date +%s%N)\",\"seq\":1,\"ts\":\"$$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)\",\"uptime_ms\":1000,\"simulated\":true,\"value\":32.5,\"valid\":true,\"unit\":\"lux\"}"
	@echo "Published development sensor readings to $(MQTT_TOPIC_PREFIX)/sensor/*"
