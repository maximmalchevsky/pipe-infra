package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/maximmalchevsky/pipe-infra/app/internal/metrics"
	"github.com/maximmalchevsky/pipe-infra/app/internal/store"
)

type Server struct {
	store   store.Store
	metrics *metrics.APIMetrics
	version string
}

func NewServer(s store.Store, m *metrics.APIMetrics, version string) *Server {
	return &Server{
		store:   s,
		metrics: m,
		version: version,
	}
}

type CreateCampaignRequest struct {
	Text       string `json:"text"`
	Recipients int    `json:"recipients"`
}

type CreateCampaignResponse struct {
	ID    int64 `json:"id"`
	Total int   `json:"total"`
}

func (s *Server) HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var req CreateCampaignRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	charCount := utf8.RuneCountInString(req.Text)
	if charCount < 1 || charCount > 160 {
		writeError(w, http.StatusBadRequest, "text must be between 1 and 160 characters")
		return
	}

	if req.Recipients < 1 || req.Recipients > 1000000 {
		writeError(w, http.StatusBadRequest, "recipients must be between 1 and 1000000")
		return
	}

	c, err := s.store.CreateCampaign(r.Context(), req.Text, req.Recipients)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create campaign")
		return
	}

	if s.metrics != nil {
		s.metrics.CampaignsCreated.Inc()
	}

	writeJSON(w, http.StatusCreated, CreateCampaignResponse{
		ID:    c.ID,
		Total: c.Total,
	})
}

func (s *Server) HandleGetCampaign(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	cd, err := s.store.GetCampaign(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "campaign not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get campaign")
		return
	}

	writeJSON(w, http.StatusOK, cd)
}

func (s *Server) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCampaigns(r.Context(), 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list campaigns")
		return
	}

	if list == nil {
		list = []store.CampaignDetail{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) HandleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
