package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

const snapshotInterval = 5 * time.Second
const readingFreshness = 30 * time.Second

// SensorSnapshotSample keeps enough of each message to explain and restore a snapshot.
type SensorSnapshotSample struct {
	ID         string    `json:"id"`
	DeviceID   string    `json:"device_id"`
	BootID     string    `json:"boot_id"`
	Seq        *int64    `json:"seq"`
	UptimeMS   *int64    `json:"uptime_ms,omitempty"`
	Simulated  *bool     `json:"simulated"`
	Valid      *bool     `json:"valid"`
	Error      *string   `json:"error,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
	ReceivedAt time.Time `json:"received_at"`
}

func (a *App) snapshotAt(at time.Time) (StoredReading, bool) {
	snapshot := StoredReading{
		ID: newUUID(), Source: "combined", RecordedAt: at, ReceivedAt: time.Now().UTC(),
		Samples: map[string]SensorSnapshotSample{},
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for sensor, reading := range a.latest {
		age := at.Sub(reading.ReceivedAt)
		if age < 0 || age >= readingFreshness {
			continue
		}
		snapshot.Samples[sensor] = SensorSnapshotSample{
			ID: reading.ID, DeviceID: reading.DeviceID, BootID: reading.BootID,
			Seq: reading.Seq, UptimeMS: reading.UptimeMS, Simulated: reading.Simulated,
			Valid: reading.Valid, Error: reading.SensorError,
			RecordedAt: reading.RecordedAt, ReceivedAt: reading.ReceivedAt,
		}
		switch sensor {
		case "door":
			snapshot.DoorOpen = reading.DoorOpen
		case "motion":
			snapshot.Motion = reading.Motion
		case "vibration":
			snapshot.Vibration = reading.Vibration
		case "distance":
			snapshot.DistanceMM = reading.DistanceMM
		case "light":
			snapshot.LightLux = reading.LightLux
		}
	}
	return snapshot, len(snapshot.Samples) > 0
}

func restoreSnapshot(snapshot StoredReading, now time.Time) map[string]StoredReading {
	latest := map[string]StoredReading{}
	for sensor, sample := range snapshot.Samples {
		age := now.Sub(sample.ReceivedAt)
		if age < 0 || age >= readingFreshness {
			continue
		}
		reading := StoredReading{
			ID: sample.ID, Sensor: sensor, DeviceID: sample.DeviceID, BootID: sample.BootID,
			Seq: sample.Seq, UptimeMS: sample.UptimeMS, Simulated: sample.Simulated,
			Valid: sample.Valid, SensorError: sample.Error,
			RecordedAt: sample.RecordedAt, ReceivedAt: sample.ReceivedAt,
		}
		switch sensor {
		case "door":
			reading.Source, reading.DoorOpen = "node", snapshot.DoorOpen
		case "motion":
			reading.Source, reading.Motion = "node", snapshot.Motion
		case "vibration":
			reading.Source, reading.Vibration = "node", snapshot.Vibration
		case "distance":
			reading.Source, reading.DistanceMM = "gateway", snapshot.DistanceMM
		case "light":
			reading.Source, reading.LightLux = "gateway", snapshot.LightLux
		default:
			continue
		}
		latest[sensor] = reading
	}
	return latest
}

func (a *App) restoreLatest(ctx context.Context) error {
	var snapshot StoredReading
	var samples []byte
	err := a.db.QueryRow(ctx, `SELECT door_open,motion,vibration,distance_mm,light_lux,samples FROM readings WHERE source='combined' ORDER BY recorded_at DESC LIMIT 1`).Scan(
		&snapshot.DoorOpen, &snapshot.Motion, &snapshot.Vibration, &snapshot.DistanceMM, &snapshot.LightLux, &samples,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(samples, &snapshot.Samples); err != nil {
		return err
	}
	a.latest = restoreSnapshot(snapshot, time.Now().UTC())
	return nil
}

func (a *App) storeSnapshot(ctx context.Context, snapshot StoredReading) error {
	samples, err := json.Marshal(snapshot.Samples)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `INSERT INTO readings(id,source,door_open,motion,vibration,distance_mm,light_lux,recorded_at,received_at,samples) VALUES($1,'combined',$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
		snapshot.ID, snapshot.DoorOpen, snapshot.Motion, snapshot.Vibration, snapshot.DistanceMM, snapshot.LightLux, snapshot.RecordedAt, snapshot.ReceivedAt, samples,
	)
	return err
}

func (a *App) logSnapshots(ctx context.Context) {
	for {
		boundary := time.Now().UTC().Truncate(snapshotInterval).Add(snapshotInterval)
		timer := time.NewTimer(time.Until(boundary))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		snapshot, ok := a.snapshotAt(boundary)
		if !ok {
			continue
		}
		if err := a.storeSnapshot(ctx, snapshot); err != nil {
			log.Printf("store reading snapshot: %v", err)
		}
	}
}
