package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnalyzePhotoGemini(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing API key")
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
