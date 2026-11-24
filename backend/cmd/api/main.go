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
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	DoorOpen     *bool     `json:"door_open"`
	LockState    *string   `json:"lock_state"`
	Presence     *bool     `json:"presence"`
	TemperatureC *float64  `json:"temperature_c"`
	DistanceMM   *float64  `json:"distance_mm"`
	LightLux     *float64  `json:"light_lux"`
	RecordedAt   time.Time `json:"recorded_at"`
	ReceivedAt   time.Time `json:"received_at"`
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
	db       *pgxpool.Pool
	mqtt     mqtt.Client
	prefix   string
	mu       sync.RWMutex
	latest   map[string]StoredReading
	lastSeen time.Time
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
	a := &App{db: pool, prefix: strings.Trim(env("MQTT_TOPIC_PREFIX", "school-demo"), "/"), latest: map[string]StoredReading{}}
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
	log.Printf("API listening on %s", env("API_ADDR", ":8080"))
	log.Fatal(http.ListenAndServe(env("API_ADDR", ":8080"), cors(r)))
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	sql, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		return err
	}
	for _, statement := range strings.Split(string(sql), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err = db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) connectMQTT() {
	clientID := env("MQTT_CLIENT_ID", "smart-door-api-local")
	opts := mqtt.NewClientOptions().AddBroker(env("MQTT_URL", "tcp://localhost:1883")).SetClientID(clientID).SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(2 * time.Second)
	a.mqtt = mqtt.NewClient(opts)
	for {
		token := a.mqtt.Connect()
		if token.Wait() && token.Error() == nil {
			break
		}
		log.Printf("MQTT unavailable; retrying: %v", token.Error())
		time.Sleep(2 * time.Second)
	}
	for _, topic := range []string{"door/node/state", "door/gateway/telemetry", "door/cmd/result"} {
		if token := a.mqtt.Subscribe(a.topic(topic), 1, a.handleMQTT); token.Wait() && token.Error() != nil {
			log.Printf("subscribe %s: %v", topic, token.Error())
		}
	}
}
func (a *App) topic(s string) string { return a.prefix + "/" + s }
func (a *App) handleMQTT(_ mqtt.Client, m mqtt.Message) {
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
	var reading StoredReading
	if json.Unmarshal(m.Payload(), &reading) != nil || (reading.Source != "node" && reading.Source != "gateway") || reading.ID == "" {
		return
	}
	if reading.RecordedAt.IsZero() {
		reading.RecordedAt = time.Now().UTC()
	}
	reading.ReceivedAt = time.Now().UTC()
	_, err := a.db.Exec(context.Background(), `INSERT INTO readings(id,source,door_open,lock_state,presence,temperature_c,distance_mm,light_lux,recorded_at,received_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, reading.ID, reading.Source, reading.DoorOpen, reading.LockState, reading.Presence, reading.TemperatureC, reading.DistanceMM, reading.LightLux, reading.RecordedAt, reading.ReceivedAt)
	if err != nil {
		log.Printf("store reading: %v", err)
		return
	}
	a.mu.Lock()
	a.latest[reading.Source] = reading
	a.lastSeen = reading.ReceivedAt
	a.mu.Unlock()
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
	node, hasNode := a.latest["node"]
	online := hasNode && time.Since(node.ReceivedAt) < 30*time.Second
	a.mu.RUnlock()
	jsonResponse(w, http.StatusOK, map[string]any{"door_id": "door-01", "online": online, "readings": values})
}
func (a *App) readings(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	rows, err := a.db.Query(r.Context(), `SELECT id::text,source,door_open,lock_state,presence,temperature_c,distance_mm,light_lux,recorded_at,received_at FROM readings WHERE ($1='' OR recorded_at >= $1::timestamptz) AND ($2='' OR recorded_at <= $2::timestamptz) ORDER BY recorded_at DESC LIMIT $3`, from, to, limit)
	if err != nil {
		writeError(w, 500, "could not read history")
		return
	}
	defer rows.Close()
	result := []StoredReading{}
	for rows.Next() {
		var x StoredReading
		if err = rows.Scan(&x.ID, &x.Source, &x.DoorOpen, &x.LockState, &x.Presence, &x.TemperatureC, &x.DistanceMM, &x.LightLux, &x.RecordedAt, &x.ReceivedAt); err != nil {
			writeError(w, 500, "could not read history")
			return
		}
		result = append(result, x)
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
	node, hasNode := a.latest["node"]
	online := hasNode && time.Since(node.ReceivedAt) < 30*time.Second
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
	payload, _ := json.Marshal(map[string]any{"id": id, "action": body.Action, "expires_at": c.ExpiresAt})
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
