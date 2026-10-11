package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maximmalchevsky/pipe-infra/app/internal/api"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
)

func setupTestRouter(fake *store.FakeStore) http.Handler {
	return api.NewRouter(fake, nil, "test-version", 0, 0.0)
}

func TestValidation_CreateCampaign(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid json",
			body:       "{invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty text",
			body:       `{"text":"","recipients":100}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "text too long (>160 chars)",
			body:       `{"text":"` + string(make([]byte, 161)) + `","recipients":100}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "recipients zero",
			body:       `{"text":"promo","recipients":0}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "recipients negative",
			body:       `{"text":"promo","recipients":-5}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "recipients too large (>1000000)",
			body:       `{"text":"promo","recipients":1000001}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/campaigns", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}

			var errResp map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("expected JSON error response: %v", err)
			}
			if errResp["error"] == "" {
				t.Error("expected non-empty error field")
			}
		})
	}
}

func TestCreateCampaign_Success201(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	body := `{"text":"Hello World","recipients":500}`
	req := httptest.NewRequest("POST", "/api/v1/campaigns", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp api.CreateCampaignResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID <= 0 {
		t.Errorf("expected positive campaign ID, got %d", resp.ID)
	}
	if resp.Total != 500 {
		t.Errorf("expected total 500, got %d", resp.Total)
	}
}

func TestGetCampaign_NotFound404(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	req := httptest.NewRequest("GET", "/api/v1/campaigns/9999", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["error"] != "campaign not found" {
		t.Errorf("expected 'campaign not found', got %q", errResp["error"])
	}
}

func TestGetCampaign_Success(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	c, err := fake.CreateCampaign(t.Context(), "Test message", 50)
	if err != nil {
		t.Fatalf("failed to create test campaign: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/campaigns/1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var detail store.CampaignDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode detail: %v", err)
	}
	if detail.ID != c.ID || detail.Total != 50 || detail.Pending != 50 {
		t.Errorf("detail mismatch: %+v", detail)
	}
}

func TestReadyz_PingFailure503(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	// Normal readyz -> 200
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Ping failure -> 503
	fake.SetPingErr(errors.New("db connection refused"))
	req = httptest.NewRequest("GET", "/readyz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "not ready" || resp["error"] == "" {
		t.Errorf("expected status 'not ready' with error, got: %+v", resp)
	}
}

func TestHealthz_Always200(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestVersion_200(t *testing.T) {
	fake := store.NewFakeStore()
	handler := setupTestRouter(fake)

	req := httptest.NewRequest("GET", "/version", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["version"] != "test-version" {
		t.Errorf("expected test-version, got %s", resp["version"])
	}
}
