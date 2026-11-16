// Package api is the HTTP layer: routing, validation and status codes. No SQL here.
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/saranshrajput/likecounter/internal/likes"
)

type Handler struct {
	svc *likes.Service
}

// New returns the router. Go 1.22+ ServeMux supports "METHOD /path/{param}" patterns.
func New(svc *likes.Service) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /posts/{postId}", h.getPost)
	mux.HandleFunc("POST /posts/{postId}/like", h.like)
	mux.HandleFunc("DELETE /posts/{postId}/like", h.unlike)
	return mux
}

type likeRequest struct {
	UserID string `json:"userId"`
}

func (h *Handler) getPost(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetPost(r.Context(), r.PathValue("postId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) like(w http.ResponseWriter, r *http.Request) {
	userID, ok := readUserID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.LikePost(r.Context(), r.PathValue("postId"), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, res)
}

func (h *Handler) unlike(w http.ResponseWriter, r *http.Request) {
	userID, ok := readUserID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.UnlikePost(r.Context(), r.PathValue("postId"), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func readUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req likeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.UserID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userId must not be blank"})
		return "", false
	}
	return req.UserID, true
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, likes.ErrPostNotFound), errors.Is(err, likes.ErrUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		log.Printf("internal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
