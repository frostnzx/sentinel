package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestAnalyzePhotoGemini(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing API key")
		}
		var request struct {
			GenerationConfig struct {
				ResponseMIMEType   string `json:"responseMimeType"`
				ResponseJSONSchema struct {
					Type     string   `json:"type"`
					Required []string `json:"required"`
				} `json:"responseJsonSchema"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Gemini request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		config := request.GenerationConfig
		schema := config.ResponseJSONSchema
		if config.ResponseMIMEType != "application/json" || schema.Type != "object" ||
			!slices.Equal(schema.Required, []string{"category", "description", "people_count"}) {
			t.Errorf("unexpected Gemini response configuration: %+v", config)
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"category\":\"person_visible\",\"description\":\"One person\",\"people_count\":1}"}]}}]}`))
	}))
	defer server.Close()
	t.Setenv("VISITOR_ANALYSIS_MODE", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_API_BASE_URL", server.URL)
	result, err := analyzePhoto(context.Background(), []byte("image"), "image/jpeg")
	if err != nil || result.Category != "person_visible" || result.PeopleCount == nil || *result.PeopleCount != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestAnalyzePhotoRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"category\":\"invalid\"}"}]}}]}`))
	}))
	defer server.Close()
	t.Setenv("VISITOR_ANALYSIS_MODE", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_API_BASE_URL", server.URL)
	if _, err := analyzePhoto(context.Background(), nil, "image/jpeg"); err == nil {
		t.Fatal("expected invalid response error")
	}
}
