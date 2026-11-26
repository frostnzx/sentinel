package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type faceImage struct {
	Name string
	Data []byte
}
type embedResult struct {
	Filename  string    `json:"filename"`
	Error     string    `json:"error,omitempty"`
	Embedding []float64 `json:"embedding,omitempty"`
}
type embedResponse struct {
	ModelVersion string        `json:"model_version"`
	Results      []embedResult `json:"results"`
}

func (a *App) createPerson(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		writeError(w, 400, "name is required")
		return
	}
	id := newUUID()
	name := strings.TrimSpace(input.Name)
	if _, err := a.db.Exec(r.Context(), `INSERT INTO trusted_people(id,name) VALUES($1,$2)`, id, name); err != nil {
		writeError(w, 500, "could not create person")
		return
	}
	jsonResponse(w, 201, map[string]string{"id": id, "name": name})
}

func (a *App) listPeople(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	var version int64
	var applied *int64
	var syncError string
	err := a.db.QueryRow(r.Context(), `SELECT snapshot,version,applied_version,error FROM face_sync WHERE singleton=true`).Scan(&raw, &version, &applied, &syncError)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not read gallery")
		return
	}
	faces := []facePerson{}
	if len(raw) > 0 && json.Unmarshal(raw, &faces) != nil {
		writeError(w, 500, "invalid gallery")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT id::text,name,deleted_version FROM trusted_people ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, 500, "could not list people")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	seen := map[string]bool{}
	for rows.Next() {
		var id, name string
		var deleted *int64
		if rows.Scan(&id, &name, &deleted) != nil {
			writeError(w, 500, "could not list people")
			return
		}
		seen[id] = true
		if deleted != nil && applied != nil && *applied >= *deleted {
			continue
		}
		enrolled := false
		for _, face := range faces {
			if face.ID == id {
				enrolled = true
				break
			}
		}
		result = append(result, map[string]any{"id": id, "name": name, "enrolled": enrolled, "deleted": deleted != nil, "version": version, "applied_version": applied, "sync_error": syncError})
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not list people")
		return
	}
	for _, face := range faces {
		if !seen[face.ID] {
			result = append(result, map[string]any{"id": face.ID, "name": face.Name, "enrolled": true, "deleted": false, "version": version, "applied_version": applied, "sync_error": syncError})
		}
	}
	jsonResponse(w, 200, result)
}

func (a *App) startEnrollment(w http.ResponseWriter, r *http.Request) {
	personID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid person ID")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 26*1024*1024)
	if r.ParseMultipartForm(5*1024*1024) != nil {
		writeError(w, 400, "invalid upload")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["images"]
	if len(files) < 3 || len(files) > 5 {
		writeError(w, 400, "upload 3–5 photos")
		return
	}
	images := make([]faceImage, 0, len(files))
	for _, header := range files {
		file, e := header.Open()
		if e != nil {
			writeError(w, 400, "could not read photo")
			return
		}
		data, e := io.ReadAll(io.LimitReader(file, 5*1024*1024+1))
		file.Close()
		mime := http.DetectContentType(data)
		if e != nil || len(data) > 5*1024*1024 || (mime != "image/jpeg" && mime != "image/png") {
			writeError(w, 400, "photos must be JPEG or PNG under 5 MiB")
			return
		}
		images = append(images, faceImage{Name: header.Filename, Data: data})
	}
	id := newUUID()
	tag, err := a.db.Exec(r.Context(), `INSERT INTO face_enrollments(id,person_id,status) SELECT $1,id,'pending' FROM trusted_people WHERE id=$2 AND deleted_version IS NULL`, id, personID.String())
	if err != nil {
		writeError(w, 500, "could not start enrollment")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "person not found")
		return
	}
	go a.processEnrollment(id, personID.String(), images)
	jsonResponse(w, 202, map[string]string{"id": id, "status": "pending"})
}

func (a *App) processEnrollment(id, personID string, images []faceImage) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	fail := func(message string, results any) {
		details, _ := json.Marshal(results)
		if _, err := a.db.Exec(context.Background(), `UPDATE face_enrollments SET status='failed',error=$2,results=$3 WHERE id=$1 AND status='pending'`, id, message, details); err != nil {
			log.Printf("save enrollment failure: %v", err)
		}
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, image := range images {
		part, err := writer.CreateFormFile("images", image.Name)
		if err != nil {
			fail("Could not prepare photos", nil)
			return
		}
		if _, err = part.Write(image.Data); err != nil {
			fail("Could not prepare photos", nil)
			return
		}
	}
	if writer.Close() != nil {
		fail("Could not prepare photos", nil)
		return
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(env("FACE_AI_URL", "http://face-ai:8000"), "/")+"/ai/faces/embed", &body)
	if err != nil {
		fail("Face service unavailable", nil)
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("Face service unavailable", nil)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		fail("Face service failed", nil)
		return
	}
	var result embedResponse
	if json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(&result) != nil || result.ModelVersion == "" || len(result.Results) != len(images) {
		fail("Invalid face service response", nil)
		return
	}
	embeddings := [][]float64{}
	for _, item := range result.Results {
		if item.Error == "" && len(item.Embedding) == 128 {
			embeddings = append(embeddings, item.Embedding)
		}
	}
	for i := range result.Results {
		result.Results[i].Embedding = nil
	}
	if len(embeddings) < 2 {
		fail("At least two photos must be accepted", result.Results)
		return
	}
	if err = a.saveEnrollment(ctx, id, personID, result, embeddings); err != nil {
		fail(err.Error(), result.Results)
		return
	}
	a.publishFaceSnapshot()
}

func (a *App) saveEnrollment(ctx context.Context, id, personID string, result embedResponse, embeddings [][]float64) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(772311)`); err != nil {
		return err
	}
	var name string
	if err = tx.QueryRow(ctx, `SELECT name FROM trusted_people WHERE id=$1 AND deleted_version IS NULL FOR UPDATE`, personID).Scan(&name); err != nil {
		return errors.New("Person unavailable")
	}
	var model string
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT model_version,snapshot FROM face_sync WHERE singleton=true FOR UPDATE`).Scan(&model, &raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	faces := []facePerson{}
	if len(raw) > 0 && json.Unmarshal(raw, &faces) != nil {
		return errors.New("Invalid gallery")
	}
	if len(faces) > 0 && model != result.ModelVersion {
		return errors.New("Face model version does not match gallery")
	}
	updated := []facePerson{}
	for _, face := range faces {
		if face.ID != personID {
			updated = append(updated, face)
		}
	}
	updated = append(updated, facePerson{ID: personID, Name: name, Embeddings: embeddings})
	gallery, _ := json.Marshal(updated)
	if _, err = tx.Exec(ctx, `INSERT INTO face_sync(singleton,version,model_version,snapshot) VALUES(true,1,$1,$2) ON CONFLICT(singleton) DO UPDATE SET version=face_sync.version+1,model_version=EXCLUDED.model_version,snapshot=EXCLUDED.snapshot,error='',updated_at=now()`, result.ModelVersion, gallery); err != nil {
		return err
	}
	details, _ := json.Marshal(result.Results)
	if _, err = tx.Exec(ctx, `UPDATE face_enrollments SET status='completed',results=$2 WHERE id=$1`, id, details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) enrollmentStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid enrollment ID")
		return
	}
	var status, message string
	var results json.RawMessage
	err = a.db.QueryRow(r.Context(), `SELECT status,error,results FROM face_enrollments WHERE id=$1`, id.String()).Scan(&status, &message, &results)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "enrollment not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not read enrollment")
		return
	}
	jsonResponse(w, 200, map[string]any{"id": id.String(), "status": status, "error": message, "results": results})
}

func (a *App) deletePerson(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid person ID")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeError(w, 500, "could not remove person")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(772311)`); err != nil {
		writeError(w, 500, "could not remove person")
		return
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT snapshot FROM face_sync WHERE singleton=true FOR UPDATE`).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not read gallery")
		return
	}
	faces := []facePerson{}
	if len(raw) > 0 && json.Unmarshal(raw, &faces) != nil {
		writeError(w, 500, "invalid gallery")
		return
	}
	kept := []facePerson{}
	found := false
	name := ""
	for _, face := range faces {
		if face.ID == id.String() {
			found = true
			name = face.Name
		} else {
			kept = append(kept, face)
		}
	}
	var existingName string
	personErr := tx.QueryRow(ctx, `SELECT name FROM trusted_people WHERE id=$1 AND deleted_version IS NULL FOR UPDATE`, id.String()).Scan(&existingName)
	if personErr != nil && !errors.Is(personErr, pgx.ErrNoRows) {
		writeError(w, 500, "could not remove person")
		return
	}
	if !found && errors.Is(personErr, pgx.ErrNoRows) {
		writeError(w, 404, "person not found")
		return
	}
	if !found {
		_, err = tx.Exec(ctx, `DELETE FROM face_enrollments WHERE person_id=$1`, id.String())
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM trusted_people WHERE id=$1`, id.String())
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			writeError(w, 500, "could not remove person")
			return
		}
		w.WriteHeader(204)
		return
	}
	if existingName != "" {
		name = existingName
	}
	gallery, _ := json.Marshal(kept)
	var version int64
	err = tx.QueryRow(ctx, `UPDATE face_sync SET version=version+1,snapshot=$1,error='',updated_at=now() WHERE singleton=true RETURNING version`, gallery).Scan(&version)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO trusted_people(id,name,deleted_version) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET deleted_version=$3`, id.String(), name, version)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		writeError(w, 500, "could not remove person")
		return
	}
	a.publishFaceSnapshot()
	jsonResponse(w, 202, map[string]any{"version": version})
}
