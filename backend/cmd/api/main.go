package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StoredReading struct {
	ID           string                          `json:"id"`
	Source       string                          `json:"source"`
	Sensor       string                          `json:"sensor,omitempty"`
	DeviceID     string                          `json:"device_id,omitempty"`
	BootID       string                          `json:"boot_id,omitempty"`
	Seq          *int64                          `json:"seq,omitempty"`
	UptimeMS     *int64                          `json:"uptime_ms,omitempty"`
	Simulated    *bool                           `json:"simulated,omitempty"`
	Valid        *bool                           `json:"valid,omitempty"`
	SensorError  *string                         `json:"error,omitempty"`
	Payload      json.RawMessage                 `json:"payload,omitempty"`
	Samples      map[string]SensorSnapshotSample `json:"samples,omitempty"`
	DoorOpen     *bool                           `json:"door_open"`
	LockState    *string                         `json:"lock_state"`
	Presence     *bool                           `json:"presence"`
	Motion       *bool                           `json:"motion"`
	Vibration    *bool                           `json:"vibration"`
	TemperatureC *float64                        `json:"temperature_c"`
	DistanceMM   *float64                        `json:"distance_mm"`
	LightLux     *float64                        `json:"light_lux"`
	RecordedAt   time.Time                       `json:"recorded_at"`
	ReceivedAt   time.Time                       `json:"received_at"`
}
type DoorCommand struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type App struct {
	db     *pgxpool.Pool
	mqtt   mqtt.Client
	prefix string
	mu     sync.RWMutex
	latest map[string]StoredReading
}

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://smartdoor:local-dev-only@localhost:5432/smartdoor?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	prefix := strings.Trim(os.Getenv("MQTT_TOPIC_PREFIX"), "/")
	if len(prefix) < 12 || prefix == "school-demo" || strings.Contains(prefix, "replace") || strings.ContainsAny(prefix, "+# ") {
		log.Fatal("set a unique MQTT_TOPIC_PREFIX")
	}
	a := &App{db: pool, prefix: prefix, latest: map[string]StoredReading{}}
	if err := a.restoreLatest(ctx); err != nil {
		log.Fatal(err)
	}
	go a.logSnapshots(ctx)
	a.connectMQTT()
	r := chi.NewRouter()
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/health/ready", a.ready)
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Smart Door API</title></head><body><script id="api-reference" data-url="/openapi.json"></script><script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script></body></html>`))
	})
	r.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "contracts/openapi.json") })
	r.Get("/api/v1/devices/door-01/state", a.state)
	r.Get("/api/v1/devices/door-01/readings", a.readings)
	r.Post("/api/v1/devices/door-01/commands", a.createCommand)
	r.Get("/api/v1/commands/{id}", a.getCommand)
	r.Get("/api/v1/devices/door-01/events", a.events)
	r.Get("/api/v1/faces/sync", a.faceStatus)
	r.Post("/api/v1/faces/sync", a.syncFaces)
	log.Printf("API listening on %s", env("API_ADDR", ":8080"))
	log.Fatal(http.ListenAndServe(env("API_ADDR", ":8080"), cors(r)))
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return err
	}
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		for _, statement := range strings.Split(string(sql), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err = db.Exec(ctx, statement); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
		}
	}
	return nil
}
func (a *App) connectMQTT() {
	clientID := env("MQTT_CLIENT_ID", "smart-door-api-local")
	opts := mqtt.NewClientOptions().AddBroker(env("MQTT_URL", "ssl://test.mosquitto.org:8886")).SetClientID(clientID).SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(2 * time.Second)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		for _, topic := range []string{"sensor/motion", "sensor/door", "sensor/distance", "sensor/light", "sensor/vibration", "door/node/alert", "door/access/event", "door/cmd/result", "door/faces/status"} {
			if token := c.Subscribe(a.topic(topic), 1, a.handleMQTT); token.Wait() && token.Error() != nil {
				log.Printf("subscribe %s: %v", topic, token.Error())
			}
		}
		a.publishFaceSnapshot()
	})
	a.mqtt = mqtt.NewClient(opts)
	go func() {
		token := a.mqtt.Connect()
		token.Wait()
		if token.Error() != nil {
			log.Printf("MQTT connect: %v", token.Error())
		}
	}()
}
func (a *App) topic(s string) string { return a.prefix + "/" + s }
func (a *App) handleMQTT(_ mqtt.Client, m mqtt.Message) {
	if strings.HasSuffix(m.Topic(), "/faces/status") {
		a.handleFaceStatus(m.Payload())
		return
	}
	if strings.HasSuffix(m.Topic(), "/access/event") || strings.HasSuffix(m.Topic(), "/node/alert") {
		a.handleEvent(m.Topic(), m.Payload())
		return
	}
	if strings.HasSuffix(m.Topic(), "/cmd/result") {
		var result struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if json.Unmarshal(m.Payload(), &result) != nil || result.ID == "" {
			return
		}
		status := result.Status
		if status != "succeeded" && status != "failed" {
			status = "failed"
		}
		_, err := a.db.Exec(context.Background(), `UPDATE commands SET status=$2,message=$3 WHERE id=$1 AND status='pending'`, result.ID, status, result.Message)
		if err != nil {
			log.Printf("store command result: %v", err)
		}
		return
	}
	if !strings.HasPrefix(m.Topic(), a.topic("sensor/")) {
		return
	}
	reading, err := parseSensorReading(strings.TrimPrefix(m.Topic(), a.topic("sensor/")), m.Payload(), time.Now().UTC())
	if err != nil {
		log.Printf("invalid sensor reading on %s: %v", m.Topic(), err)
		return
	}
	a.updateLatest(reading)
}
func (a *App) updateLatest(reading StoredReading) {
	a.mu.Lock()
	defer a.mu.Unlock()
	previous, ok := a.latest[reading.Sensor]
	if !ok || (previous.DeviceID == reading.DeviceID && previous.BootID == reading.BootID && previous.Seq != nil && reading.Seq != nil && *reading.Seq > *previous.Seq) || ((previous.DeviceID != reading.DeviceID || previous.BootID != reading.BootID) && reading.ReceivedAt.After(previous.ReceivedAt)) {
		a.latest[reading.Sensor] = reading
	}
}

// nodeOnlineLocked reads latest while the caller holds a.mu.
func (a *App) nodeOnlineLocked() bool {
	for _, reading := range a.latest {
		if reading.Source == "node" && time.Since(reading.ReceivedAt) < 30*time.Second {
			return true
		}
	}
	return false
}
func (a *App) ready(w http.ResponseWriter, r *http.Request) {
	if a.db.Ping(r.Context()) != nil || a.mqtt == nil || !a.mqtt.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "dependency unavailable")
		return
	}
	w.WriteHeader(http.StatusOK)
}
func (a *App) state(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	values := make([]StoredReading, 0, len(a.latest))
	for _, v := range a.latest {
		values = append(values, v)
	}
	online := a.nodeOnlineLocked()
	a.mu.RUnlock()
	jsonResponse(w, http.StatusOK, map[string]any{"door_id": "door-01", "online": online, "mqtt_connected": a.mqtt != nil && a.mqtt.IsConnected(), "readings": values})
}
func (a *App) readings(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	from, to, source := r.URL.Query().Get("from"), r.URL.Query().Get("to"), r.URL.Query().Get("source")
	if source != "" && source != "combined" {
		writeError(w, 400, "source must be combined")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT id::text,source,door_open,lock_state,presence,temperature_c,distance_mm,light_lux,recorded_at,received_at,COALESCE(sensor,''),COALESCE(device_id,''),COALESCE(boot_id,''),seq,uptime_ms,simulated,valid,motion,vibration,sensor_error,payload,samples FROM readings WHERE ($1='' OR recorded_at >= $1::timestamptz) AND ($2='' OR recorded_at <= $2::timestamptz) AND ($4='' OR source=$4) ORDER BY recorded_at DESC LIMIT $3`, from, to, limit, source)
	if err != nil {
		writeError(w, 500, "could not read history")
		return
	}
	defer rows.Close()
	result := []StoredReading{}
	for rows.Next() {
		var x StoredReading
		var samples []byte
		if err = rows.Scan(&x.ID, &x.Source, &x.DoorOpen, &x.LockState, &x.Presence, &x.TemperatureC, &x.DistanceMM, &x.LightLux, &x.RecordedAt, &x.ReceivedAt, &x.Sensor, &x.DeviceID, &x.BootID, &x.Seq, &x.UptimeMS, &x.Simulated, &x.Valid, &x.Motion, &x.Vibration, &x.SensorError, &x.Payload, &samples); err != nil {
			writeError(w, 500, "could not read history")
			return
		}
		if len(samples) > 0 && json.Unmarshal(samples, &x.Samples) != nil {
			writeError(w, 500, "could not read history")
			return
		}
		x.RecordedAt = x.RecordedAt.UTC()
		x.ReceivedAt = x.ReceivedAt.UTC()
		result = append(result, x)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read history")
		return
	}
	jsonResponse(w, http.StatusOK, result)
}
func (a *App) createCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || (body.Action != "lock" && body.Action != "unlock") {
		writeError(w, 400, "action must be lock or unlock")
		return
	}
	a.mu.RLock()
	online := a.nodeOnlineLocked()
	a.mu.RUnlock()
	if !online || a.mqtt == nil || !a.mqtt.IsConnected() {
		writeError(w, 503, "door device is offline")
		return
	}
	now := time.Now().UTC()
	id := newUUID()
	c := DoorCommand{ID: id, Action: body.Action, Status: "pending", CreatedAt: now, ExpiresAt: now.Add(10 * time.Second)}
	_, err := a.db.Exec(r.Context(), `INSERT INTO commands(id,action,status,created_at,expires_at) VALUES($1,$2,'pending',$3,$4)`, id, body.Action, c.CreatedAt, c.ExpiresAt)
	if err != nil {
		writeError(w, 500, "could not store command")
		return
	}
	payload, _ := json.Marshal(map[string]any{"id": id, "action": body.Action, "origin": "manual", "expires_at": c.ExpiresAt})
	t := a.mqtt.Publish(a.topic("door/cmd/lock"), 1, false, payload)
	if !t.WaitTimeout(2*time.Second) || t.Error() != nil {
		_, _ = a.db.Exec(r.Context(), `UPDATE commands SET status='failed',message='publish failed' WHERE id=$1 AND status='pending'`, id)
		writeError(w, 503, "could not publish command")
		return
	}
	jsonResponse(w, http.StatusAccepted, c)
}
func (a *App) getCommand(w http.ResponseWriter, r *http.Request) {
	var c DoorCommand
	err := a.db.QueryRow(r.Context(), `SELECT id::text,action,status,message,created_at,expires_at FROM commands WHERE id=$1`, chi.URLParam(r, "id")).Scan(&c.ID, &c.Action, &c.Status, &c.Message, &c.CreatedAt, &c.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "command not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not read command")
		return
	}
	if c.Status == "pending" && time.Now().After(c.ExpiresAt) {
		_, _ = a.db.Exec(r.Context(), `UPDATE commands SET status='timed_out' WHERE id=$1 AND status='pending'`, c.ID)
		c.Status = "timed_out"
	}
	jsonResponse(w, 200, c)
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func newUUID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
