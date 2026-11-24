package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func parseSensorReading(sensor string, payload []byte, received time.Time) (StoredReading, error) {
	switch sensor {
	case "motion", "door", "distance", "light", "vibration":
	default:
		return StoredReading{}, errors.New("unknown sensor")
	}
	var message struct {
		DeviceID  string          `json:"device_id"`
		BootID    string          `json:"boot_id"`
		Seq       *int64          `json:"seq"`
		TS        *time.Time      `json:"ts"`
		UptimeMS  *int64          `json:"uptime_ms"`
		Simulated *bool           `json:"simulated"`
		Valid     *bool           `json:"valid"`
		Value     json.RawMessage `json:"value"`
		Raw       *int            `json:"raw"`
		Unit      string          `json:"unit"`
		Error     string          `json:"error"`
	}
	if err := json.Unmarshal(payload, &message); err != nil {
		return StoredReading{}, err
	}
	if message.DeviceID == "" || message.BootID == "" || message.Seq == nil || *message.Seq < 0 || message.Valid == nil || message.Simulated == nil || len(message.Value) == 0 {
		return StoredReading{}, errors.New("missing or invalid sensor metadata")
	}
	if message.UptimeMS != nil && *message.UptimeMS < 0 {
		return StoredReading{}, errors.New("negative uptime")
	}
	if message.TS != nil && message.TS.IsZero() {
		return StoredReading{}, errors.New("invalid timestamp")
	}
	key := []byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", sensor, message.DeviceID, message.BootID, *message.Seq))
	hash := sha256.Sum256(key)
	hash[6] = (hash[6] & 0x0f) | 0x40
	hash[8] = (hash[8] & 0x3f) | 0x80
	reading := StoredReading{
		ID:     fmt.Sprintf("%x-%x-%x-%x-%x", hash[:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16]),
		Sensor: sensor, DeviceID: message.DeviceID, BootID: message.BootID, Seq: message.Seq,
		UptimeMS: message.UptimeMS, Simulated: message.Simulated, Valid: message.Valid,
		Payload: append(json.RawMessage(nil), payload...), RecordedAt: received, ReceivedAt: received,
	}
	if message.TS != nil {
		reading.RecordedAt = message.TS.UTC()
	}
	if message.Error != "" {
		reading.SensorError = &message.Error
	}
	if sensor == "distance" || sensor == "light" {
		reading.Source = "gateway"
		if message.TS == nil {
			return StoredReading{}, errors.New("gateway timestamp is required")
		}
	} else {
		reading.Source = "node"
	}
	if !*message.Valid {
		// Reed calibration is unfinished, but the hardware raw contact bit still
		// reports the requested position: 0=closed and 1=open.
		if sensor == "door" && message.Raw != nil && (*message.Raw == 0 || *message.Raw == 1) {
			open := *message.Raw == 1
			reading.DoorOpen = &open
		}
		return reading, nil
	}
	switch sensor {
	case "motion", "door", "vibration":
		var value bool
		if err := json.Unmarshal(message.Value, &value); err != nil || string(message.Value) == "null" {
			return StoredReading{}, errors.New("valid boolean sensor requires a boolean value")
		}
		switch sensor {
		case "motion":
			reading.Motion = &value
		case "door":
			open := !value // Calibrated reed value true means closed.
			reading.DoorOpen = &open
		case "vibration":
			reading.Vibration = &value
		}
	case "distance", "light":
		unit := "mm"
		if sensor == "light" {
			unit = "lux"
		}
		if message.Unit != unit {
			return StoredReading{}, fmt.Errorf("%s unit must be %s", sensor, unit)
		}
		var value float64
		if err := json.Unmarshal(message.Value, &value); err != nil || string(message.Value) == "null" {
			return StoredReading{}, errors.New("valid numeric sensor requires a number")
		}
		if sensor == "distance" {
			reading.DistanceMM = &value
		} else {
			reading.LightLux = &value
		}
	}
	return reading, nil
}
