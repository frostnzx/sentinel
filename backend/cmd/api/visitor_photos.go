package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (a *App) uploadVisitorPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024+65536)
	if err := r.ParseMultipartForm(5 * 1024 * 1024); err != nil {
		writeError(w, 400, "invalid or oversized upload")
		return
	}
	defer r.MultipartForm.RemoveAll()
	eventID, err := uuid.Parse(r.FormValue("event_id"))
	if err != nil {
		writeError(w, 400, "event_id must be a UUID")
		return
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, 400, "image is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 5*1024*1024+1))
	if err != nil || len(data) > 5*1024*1024 {
		writeError(w, 400, "image exceeds 5 MiB")
		return
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		writeError(w, 400, "image must be JPEG or PNG")
		return
	}
	id := newUUID()
	var stored string
	err = a.db.QueryRow(r.Context(), `INSERT INTO visitor_photos(id,event_id,content_type,image) VALUES($1,$2,$3,$4) ON CONFLICT(event_id) DO NOTHING RETURNING id::text`, id, eventID.String(), "image/"+format, data).Scan(&stored)
	status := http.StatusCreated
	if errors.Is(err, pgx.ErrNoRows) {
		status = http.StatusOK
		err = a.db.QueryRow(r.Context(), `SELECT id::text FROM visitor_photos WHERE event_id=$1`, eventID.String()).Scan(&stored)
	}
	if err != nil {
		writeError(w, 500, "could not store photo")
		return
	}
	jsonResponse(w, status, map[string]string{"id": stored})
}

func (a *App) visitorPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid photo ID")
		return
	}
	var data []byte
	var contentType string
	err = a.db.QueryRow(r.Context(), `SELECT content_type,image FROM visitor_photos WHERE id=$1 AND created_at>now()-interval '30 days'`, id.String()).Scan(&contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "photo not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not read photo")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

type visitorResult struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	PeopleCount *int   `json:"people_count"`
	Simulated   bool   `json:"simulated,omitempty"`
}

func analyzePhoto(ctx context.Context, data []byte, mime string) (visitorResult, error) {
	if env("VISITOR_ANALYSIS_MODE", "off") == "stub" {
		return visitorResult{Category: "unclear", Description: "Simulated analysis", Simulated: true}, nil
	}
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return visitorResult{}, errors.New("Gemini API key is missing")
	}
	body := map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "Describe only visible details. Return JSON with category (person_visible, no_person_visible, or unclear), description, and people_count (integer or null)."}, map[string]any{"inline_data": map[string]string{"mime_type": mime, "data": base64.StdEncoding.EncodeToString(data)}}}}}, "generationConfig": map[string]any{"responseMimeType": "application/json"}}
	encoded, err := json.Marshal(body)
	if err != nil {
		return visitorResult{}, err
	}
	endpoint := strings.TrimRight(env("GEMINI_API_BASE_URL", "https://generativelanguage.googleapis.com"), "/") + "/v1beta/models/" + env("GEMINI_MODEL", "gemini-3.1-flash-lite") + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return visitorResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return visitorResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return visitorResult{}, fmt.Errorf("Gemini returned HTTP %d", resp.StatusCode)
	}
	var output struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&output); err != nil {
		return visitorResult{}, err
	}
	if len(output.Candidates) == 0 || len(output.Candidates[0].Content.Parts) == 0 {
		return visitorResult{}, errors.New("empty Gemini response")
	}
	var result visitorResult
	if err = json.Unmarshal([]byte(output.Candidates[0].Content.Parts[0].Text), &result); err != nil {
		return result, err
	}
	if (result.Category != "person_visible" && result.Category != "no_person_visible" && result.Category != "unclear") || result.Description == "" || (result.PeopleCount != nil && *result.PeopleCount < 0) {
		return result, errors.New("invalid Gemini response")
	}
	return result, nil
}

func (a *App) visitorWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := a.db.Exec(ctx, `UPDATE visitor_photos SET analysis_status='failed',analysis_result='{"category":"unclear","description":"Analysis interrupted","people_count":null}'::jsonb WHERE analysis_status='running' AND analysis_started_at < now()-interval '15 seconds'`); err != nil {
			log.Printf("recover analysis: %v", err)
		}
		if _, err := a.db.Exec(ctx, `DELETE FROM visitor_photos WHERE created_at <= now()-interval '30 days'`); err != nil {
			log.Printf("expire photos: %v", err)
		}
		a.processVisitorPhoto(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *App) processVisitorPhoto(ctx context.Context) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(128963010)`); err != nil {
		return
	}
	var id, mime string
	var data []byte
	err = tx.QueryRow(ctx, `SELECT p.id::text,p.content_type,p.image FROM visitor_photos p JOIN events e ON e.id=p.event_id WHERE e.source='gateway' AND e.kind IN ('lockout','spoof','loiter') AND p.analysis_status IS NULL ORDER BY p.created_at FOR UPDATE OF p SKIP LOCKED LIMIT 1`).Scan(&id, &mime, &data)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("find photo: %v", err)
		}
		return
	}
	mode := env("VISITOR_ANALYSIS_MODE", "off")
	var recent bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM visitor_photos WHERE analysis_started_at > now()-interval '30 seconds')`).Scan(&recent); err != nil {
		return
	}
	if mode == "off" || recent {
		_, err = tx.Exec(ctx, `UPDATE visitor_photos SET analysis_status='skipped' WHERE id=$1`, id)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			log.Printf("skip analysis: %v", err)
		}
		return
	}
	if mode != "stub" && mode != "gemini" {
		log.Printf("invalid VISITOR_ANALYSIS_MODE")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE visitor_photos SET analysis_status='running',analysis_started_at=now() WHERE id=$1`, id); err != nil {
		return
	}
	if err = tx.Commit(ctx); err != nil {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := analyzePhoto(callCtx, data, mime)
	status := "completed"
	if err != nil {
		log.Printf("visitor analysis: %v", err)
		status = "failed"
		result = visitorResult{Category: "unclear", Description: "Analysis failed"}
	}
	output, _ := json.Marshal(result)
	if _, err = a.db.Exec(ctx, `UPDATE visitor_photos SET analysis_status=$2,analysis_result=$3 WHERE id=$1 AND analysis_status='running'`, id, status, output); err != nil {
		log.Printf("save analysis: %v", err)
	}
}
