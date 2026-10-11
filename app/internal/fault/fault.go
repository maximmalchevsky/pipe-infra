package fault

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// Middleware returns HTTP middleware applying injected latency and error rate to /api/ routes.
func Middleware(latencyMS int, errorRate float64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				if latencyMS > 0 {
					time.Sleep(time.Duration(latencyMS) * time.Millisecond)
				}

				if shouldInjectError(errorRate) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"injected fault"}`))
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func shouldInjectError(rate float64) bool {
	if rate <= 0.0 {
		return false
	}
	if rate >= 1.0 {
		return true
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return false
	}
	return float64(n.Int64())/1000000.0 < rate
}
