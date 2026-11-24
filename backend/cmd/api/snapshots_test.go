package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSnapshotAndRestore(t *testing.T) {
	at := time.Date(2026, 11, 24, 8, 19, 30, 0, time.UTC)
	boolean := func(v bool) *bool { return &v }
	number := func(v float64) *float64 { return &v }
	sequence := func(v int64) *int64 { return &v }
	valid, invalid, simulated := true, false, false
	errText := "sensor_fault"
	app := &App{latest: map[string]StoredReading{}}
	base := func(sensor, source string, seq int64) StoredReading {
		return StoredReading{ID: newUUID(), Sensor: sensor, Source: source, DeviceID: source, BootID: "boot", Seq: sequence(seq), Valid: &valid, Simulated: &simulated, RecordedAt: at.Add(-2 * time.Second), ReceivedAt: at.Add(-time.Second)}
	}
	door := base("door", "node", 1)
	door.DoorOpen, door.Valid, door.SensorError = boolean(true), &invalid, &errText
	app.updateLatest(door)
	motion := base("motion", "node", 2)
	motion.Motion = boolean(false)
	app.updateLatest(motion)
	vibration := base("vibration", "node", 3)
	vibration.Vibration = boolean(true)
	app.updateLatest(vibration)
	distance := base("distance", "gateway", 4)
	distance.DistanceMM = number(68)
	app.updateLatest(distance)
	light := base("light", "gateway", 5)
	light.LightLux = number(17.5)
	app.updateLatest(light)

	duplicate := motion
	duplicate.Motion = boolean(true)
	app.updateLatest(duplicate)
	older := motion
	older.Seq = sequence(1)
	older.Motion = boolean(true)
	app.updateLatest(older)
	if *app.latest["motion"].Motion {
		t.Fatal("duplicate or older message replaced latest motion")
	}

	snapshot, ok := app.snapshotAt(at)
	if !ok || snapshot.Source != "combined" || len(snapshot.Samples) != 5 || *snapshot.DoorOpen != true || *snapshot.Motion != false || *snapshot.Vibration != true || *snapshot.DistanceMM != 68 || *snapshot.LightLux != 17.5 {
		t.Fatalf("wrong combined snapshot: %+v", snapshot)
	}
	if snapshot.Samples["door"].Valid == nil || *snapshot.Samples["door"].Valid || *snapshot.Samples["door"].Error != errText {
		t.Fatal("invalid sensor metadata was lost")
	}
	encoded, err := json.Marshal(snapshot.Samples)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Samples = nil
	if err := json.Unmarshal(encoded, &snapshot.Samples); err != nil {
		t.Fatal(err)
	}
	restored := restoreSnapshot(snapshot, at.Add(time.Second))
	if len(restored) != 5 || restored["door"].ID != door.ID || restored["distance"].Source != "gateway" || *restored["distance"].DistanceMM != 68 || *restored["motion"].Motion {
		t.Fatalf("wrong restored sensor state: %+v", restored)
	}
	if len(restoreSnapshot(snapshot, at.Add(readingFreshness))) != 0 {
		t.Fatal("stale snapshot restored sensor state")
	}
}

func TestSnapshotSkipsStaleAndEmptyIntervals(t *testing.T) {
	at := time.Date(2026, 11, 24, 8, 19, 30, 0, time.UTC)
	app := &App{latest: map[string]StoredReading{}}
	if _, ok := app.snapshotAt(at); ok {
		t.Fatal("empty interval created a snapshot")
	}
	app.latest["motion"] = StoredReading{Sensor: "motion", ReceivedAt: at.Add(-readingFreshness)}
	if _, ok := app.snapshotAt(at); ok {
		t.Fatal("30-second-old value created a snapshot")
	}
	app.latest["motion"] = StoredReading{Sensor: "motion", ReceivedAt: at.Add(-readingFreshness + time.Nanosecond)}
	app.latest["light"] = StoredReading{Sensor: "light", ReceivedAt: at.Add(-readingFreshness)}
	snapshot, ok := app.snapshotAt(at)
	if !ok || len(snapshot.Samples) != 1 || snapshot.DistanceMM != nil || snapshot.LightLux != nil {
		t.Fatal("fresh-only snapshot did not leave missing sensors blank")
	}
	delete(app.latest, "light")
	app.latest["motion"] = StoredReading{Sensor: "motion", ReceivedAt: at.Add(time.Nanosecond)}
	if _, ok := app.snapshotAt(at); ok {
		t.Fatal("future reading entered an earlier interval")
	}
}
