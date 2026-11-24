package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type facePerson struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Embeddings [][]float64 `json:"embeddings"`
}
type faceSnapshot struct {
	Version      int64        `json:"version"`
	ModelVersion string       `json:"model_version"`
	People       []facePerson `json:"people"`
}

func (a *App) handleEvent(topic string, payload []byte) {
	var event struct {
		ID         string    `json:"id"`
		Kind       string    `json:"kind"`
		RecordedAt time.Time `json:"recorded_at"`
	}
	if json.Unmarshal(payload, &event) != nil || event.ID == "" || event.Kind == "" || event.RecordedAt.IsZero() {
		return
	}
	source := "gateway"
	if strings.HasSuffix(topic, "/node/alert") {
		source = "node"
	}
	_, err := a.db.Exec(context.Background(), `INSERT INTO events(id,source,kind,payload,recorded_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, event.ID, source, event.Kind, payload, event.RecordedAt)
	if err != nil {
		log.Printf("store event: %v", err)
	}
}

func (a *App) events(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id::text,source,kind,payload,recorded_at,received_at FROM events ORDER BY recorded_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, "could not read events")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, source, kind string
		var payload json.RawMessage
		var recorded, received time.Time
		if rows.Scan(&id, &source, &kind, &payload, &recorded, &received) != nil {
			writeError(w, 500, "could not read events")
			return
		}
		result = append(result, map[string]any{"id": id, "source": source, "kind": kind, "payload": payload, "recorded_at": recorded, "received_at": received})
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read events")
		return
	}
	jsonResponse(w, 200, result)
}

func (a *App) faceStatus(w http.ResponseWriter, r *http.Request) {
	var version int64
	var model, errText string
	var people json.RawMessage
	var applied *int64
	err := a.db.QueryRow(r.Context(), `SELECT version,model_version,snapshot,applied_version,error FROM face_sync WHERE singleton=true`).Scan(&version, &model, &people, &applied, &errText)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonResponse(w, 200, map[string]any{"version": 0, "applied_version": nil, "people": []any{}})
		return
	}
	if err != nil {
		writeError(w, 500, "could not read face sync")
		return
	}
	jsonResponse(w, 200, map[string]any{"version": version, "applied_version": applied, "model_version": model, "people": people, "error": errText})
}

func (a *App) syncFaces(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ModelVersion string       `json:"model_version"`
		People       []facePerson `json:"people"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&input) != nil || input.ModelVersion == "" || input.People == nil {
		writeError(w, 400, "model_version and people are required")
		return
	}
	for _, person := range input.People {
		if person.ID == "" || person.Name == "" || len(person.Embeddings) < 2 || len(person.Embeddings) > 5 {
			writeError(w, 400, "each person needs an ID, name, and 2-5 embeddings")
			return
		}
		for _, embedding := range person.Embeddings {
			if len(embedding) != 128 {
				writeError(w, 400, "embeddings must contain 128 numbers")
				return
			}
		}
	}
	people, _ := json.Marshal(input.People)
	var version int64
	err := a.db.QueryRow(r.Context(), `INSERT INTO face_sync(singleton,version,model_version,snapshot) VALUES(true,1,$1,$2) ON CONFLICT(singleton) DO UPDATE SET version=face_sync.version+1,model_version=EXCLUDED.model_version,snapshot=EXCLUDED.snapshot,error='',updated_at=now() RETURNING version`, input.ModelVersion, people).Scan(&version)
	if err != nil {
		writeError(w, 500, "could not store face snapshot")
		return
	}
	a.publishFaceSnapshot()
	jsonResponse(w, http.StatusAccepted, map[string]any{"version": version, "applied_version": nil, "model_version": input.ModelVersion, "people": input.People, "error": ""})
}

func (a *App) publishFaceSnapshot() {
	var snapshot faceSnapshot
	var people json.RawMessage
	err := a.db.QueryRow(context.Background(), `SELECT version,model_version,snapshot FROM face_sync WHERE singleton=true`).Scan(&snapshot.Version, &snapshot.ModelVersion, &people)
	if err != nil {
		return
	}
	if json.Unmarshal(people, &snapshot.People) != nil {
		log.Print("invalid stored face snapshot")
		return
	}
	body, _ := json.Marshal(snapshot)
	if a.mqtt == nil || !a.mqtt.IsConnected() {
		return
	}
	token := a.mqtt.Publish(a.topic("door/faces/sync"), 1, true, body)
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		log.Printf("publish face snapshot: %v", token.Error())
	}
}

func (a *App) handleFaceStatus(payload []byte) {
	var status struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(payload, &status) != nil || status.Version < 1 {
		return
	}
	if status.Status == "applied" {
		_, _ = a.db.Exec(context.Background(), `UPDATE face_sync SET applied_version=$1,error='' WHERE singleton=true AND version=$1`, status.Version)
	} else if status.Status == "error" {
		_, _ = a.db.Exec(context.Background(), `UPDATE face_sync SET error=$2 WHERE singleton=true AND version=$1`, status.Version, status.Error)
	}
}
