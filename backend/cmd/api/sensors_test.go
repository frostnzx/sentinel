package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseSensorReadings(t *testing.T) {
	received := time.Date(2026, 11, 24, 8, 19, 27, 0, time.UTC)
	tests := []struct {
		sensor, payload string
		check           func(*testing.T, StoredReading)
	}{
		{"motion", `{"device_id":"esp32-94C3C14EB580","boot_id":"DD088E1F","seq":507,"uptime_ms":169823,"simulated":false,"value":false,"valid":true,"raw":0}`, func(t *testing.T, r StoredReading) {
			if r.Source != "node" || r.Motion == nil || *r.Motion || !r.RecordedAt.Equal(received) {
				t.Fatalf("unexpected motion reading: %+v", r)
			}
		}},
		{"door", `{"device_id":"esp32-94C3C14EB580","boot_id":"DD088E1F","seq":508,"uptime_ms":170820,"simulated":false,"value":null,"valid":false,"raw":1,"meaning":"door_open","error":"reed_not_calibrated"}`, func(t *testing.T, r StoredReading) {
			if r.DoorOpen == nil || !*r.DoorOpen || r.SensorError == nil || *r.SensorError != "reed_not_calibrated" || !strings.Contains(string(r.Payload), `"raw":1`) {
				t.Fatalf("invalid door was trusted or metadata lost: %+v", r)
			}
		}},
		{"distance", `{"device_id":"pi5-gateway","boot_id":"0c0cbfaa","seq":165,"ts":"2026-11-24T08:19:26.160+00:00","uptime_ms":82049,"simulated":false,"value":68.0,"valid":true,"unit":"mm"}`, func(t *testing.T, r StoredReading) {
			if r.Source != "gateway" || r.DistanceMM == nil || *r.DistanceMM != 68 || r.RecordedAt.Nanosecond() != 160000000 || !r.ReceivedAt.Equal(received) {
				t.Fatalf("unexpected distance reading: %+v", r)
			}
		}},
		{"light", `{"device_id":"pi5-gateway","boot_id":"0c0cbfaa","seq":166,"ts":"2026-11-24T08:19:26.161+00:00","uptime_ms":82049,"simulated":false,"value":17.5,"valid":true,"unit":"lux"}`, func(t *testing.T, r StoredReading) {
			if r.LightLux == nil || *r.LightLux != 17.5 {
				t.Fatalf("unexpected light reading: %+v", r)
			}
		}},
		{"vibration", `{"device_id":"esp32-94C3C14EB580","boot_id":"DD088E1F","seq":509,"uptime_ms":170822,"simulated":false,"value":false,"valid":true,"pulses":0,"window_ms":1001}`, func(t *testing.T, r StoredReading) {
			if r.Vibration == nil || *r.Vibration || !strings.Contains(string(r.Payload), `"window_ms":1001`) {
				t.Fatalf("unexpected vibration reading: %+v", r)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.sensor, func(t *testing.T) {
			r, err := parseSensorReading(tc.sensor, []byte(tc.payload), received)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, r)
			again, err := parseSensorReading(tc.sensor, []byte(tc.payload), received.Add(time.Minute))
			if err != nil || again.ID != r.ID {
				t.Fatalf("message identity changed: %q versus %q, %v", r.ID, again.ID, err)
			}
		})
	}
	for _, tc := range []struct {
		value string
		open  bool
	}{{"true", false}, {"false", true}} {
		payload := `{"device_id":"esp32","boot_id":"boot","seq":1,"simulated":false,"valid":true,"value":` + tc.value + `}`
		r, err := parseSensorReading("door", []byte(payload), received)
		if err != nil || r.DoorOpen == nil || *r.DoorOpen != tc.open {
			t.Fatalf("door value %s: %+v, %v", tc.value, r, err)
		}
	}
	for _, tc := range []struct {
		raw  string
		open bool
	}{{"0", false}, {"1", true}} {
		payload := `{"device_id":"esp32","boot_id":"boot","seq":1,"simulated":false,"valid":false,"value":null,"raw":` + tc.raw + `,"error":"reed_not_calibrated"}`
		r, err := parseSensorReading("door", []byte(payload), received)
		if err != nil || r.DoorOpen == nil || *r.DoorOpen != tc.open {
			t.Fatalf("door raw %s: %+v, %v", tc.raw, r, err)
		}
	}
}

func TestLatestSensorSequence(t *testing.T) {
	now := time.Now().UTC()
	app := &App{latest: map[string]StoredReading{}}
	seq := func(n int64) *int64 { return &n }
	first := StoredReading{Sensor: "motion", Source: "node", DeviceID: "esp32", BootID: "boot", Seq: seq(5), ReceivedAt: now}
	app.updateLatest(first)
	app.updateLatest(StoredReading{Sensor: "door", Source: "node", DeviceID: "esp32", BootID: "boot", Seq: seq(6), ReceivedAt: now})
	app.updateLatest(StoredReading{Sensor: "motion", Source: "node", DeviceID: "esp32", BootID: "boot", Seq: seq(4), ReceivedAt: now.Add(time.Second)})
	if *app.latest["motion"].Seq != 5 || len(app.latest) != 2 || !app.nodeOnlineLocked() {
		t.Fatal("sensor slots or sequence ordering changed")
	}
	app.latest["motion"] = StoredReading{Source: "node", ReceivedAt: now.Add(-31 * time.Second)}
	app.latest["door"] = StoredReading{Source: "node", ReceivedAt: now.Add(-31 * time.Second)}
	if app.nodeOnlineLocked() {
		t.Fatal("stale node reported online")
	}
}
