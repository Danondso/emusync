package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dublin/emusync/internal/authtoken"
	"github.com/dublin/emusync/internal/model"
)

//go:embed adminweb/*
var adminWeb embed.FS

// AdminBearerMiddleware enforces Authorization: Bearer <admin token>.
func AdminBearerMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.Error(w, "admin disabled", http.StatusNotFound)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(strings.TrimSpace(auth), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}
		clientTok := authtoken.Normalize(parts[1])
		if subtle.ConstantTimeCompare([]byte(clientTok), []byte(token)) != 1 {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AdminHandler serves /admin static UI and /admin/api/v1/* JSON when adminToken is non-empty.
func (h *Handlers) AdminHandler(adminToken string) http.Handler {
	if adminToken == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	sub, err := fs.Sub(adminWeb, "adminweb")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "admin ui unavailable", http.StatusInternalServerError)
		})
	}

	mux := http.NewServeMux()
	mux.Handle("GET /admin/", serveAdminIndex(sub))
	mux.Handle("GET /admin/api/v1/status", AdminBearerMiddleware(adminToken, http.HandlerFunc(h.handleAdminStatus)))
	mux.Handle("GET /admin/api/v1/conflicts", AdminBearerMiddleware(adminToken, http.HandlerFunc(h.handleListConflicts)))
	mux.Handle("POST /admin/api/v1/conflicts/{id}/resolve", AdminBearerMiddleware(adminToken, http.HandlerFunc(h.handleResolveConflict)))
	mux.Handle("GET /admin/api/v1/profile", AdminBearerMiddleware(adminToken, http.HandlerFunc(h.handleAdminGetProfile)))
	mux.Handle("PUT /admin/api/v1/profile", AdminBearerMiddleware(adminToken, http.HandlerFunc(h.handleAdminPutProfile)))
	return mux
}

func serveAdminIndex(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/" && r.URL.Path != "/admin" {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "index missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

func (h *Handlers) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data_dir": h.storage.DataDir(),
	})
}

func (h *Handlers) handleAdminGetProfile(w http.ResponseWriter, r *http.Request) {
	doc, err := h.storage.ReadProfile()
	if err != nil {
		slog.Error("read profile", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (h *Handlers) handleAdminPutProfile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var doc ProfileDocument
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if doc.Version != 1 {
		http.Error(w, "version must be 1", http.StatusBadRequest)
		return
	}
	for _, e := range doc.Emulators {
		if err := ValidateName(e.Name); err != nil {
			http.Error(w, "invalid emulator name: "+e.Name, http.StatusBadRequest)
			return
		}
		for _, pn := range e.ProcessNames {
			if strings.TrimSpace(pn) == "" {
				http.Error(w, "empty process_names entry", http.StatusBadRequest)
				return
			}
		}
		for _, sp := range e.SavePaths {
			if strings.TrimSpace(sp) == "" {
				http.Error(w, "empty save_paths entry", http.StatusBadRequest)
				return
			}
		}
	}
	stored := ProfileDocument{
		Version:   1,
		Emulators: append([]model.EmulatorConfig(nil), doc.Emulators...),
	}
	if err := h.storage.WriteProfile(&stored); err != nil {
		slog.Error("write profile", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
