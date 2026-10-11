package fault

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestFaultMiddleware_RateZero(t *testing.T) {
	mw := Middleware(0, 0.0)
	handler := mw(okHandler())

	for i := 0; i < 50; i++ {
		req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func TestFaultMiddleware_RateOne(t *testing.T) {
	mw := Middleware(0, 1.0)
	handler := mw(okHandler())

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != `{"error":"injected fault"}` {
			t.Fatalf("unexpected body: %s", body)
		}
	}
}

func TestFaultMiddleware_HealthRoutesUnaffected(t *testing.T) {
	// Even with 100% error rate and 50ms latency, non-/api/ routes should pass immediately and succeed
	mw := Middleware(50, 1.0)
	handler := mw(okHandler())

	routes := []string{"/healthz", "/readyz", "/metrics", "/version"}
	for _, route := range routes {
		start := time.Now()
		req := httptest.NewRequest("GET", route, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("route %s: expected 200, got %d", route, rec.Code)
		}
		if elapsed := time.Since(start); elapsed > 30*time.Millisecond {
			t.Errorf("route %s: took %v, expected no injected latency", route, elapsed)
		}
	}
}

func TestFaultMiddleware_LatencyApplied(t *testing.T) {
	latency := 25
	mw := Middleware(latency, 0.0)
	handler := mw(okHandler())

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if elapsed < time.Duration(latency)*time.Millisecond {
		t.Fatalf("expected at least %dms latency, got %v", latency, elapsed)
	}
}
