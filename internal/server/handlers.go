package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dublin/emusync/internal/model"
)

const maxUploadSize = 512 << 20 // 512 MB

// Handlers holds the HTTP API route handlers.
type Handlers struct {
	storage *Storage
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(storage *Storage) *Handlers {
	return &Handlers{storage: storage}
}

// RegisterRoutes registers all API routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/manifest/{emulator}", h.handleGetManifest)
	mux.HandleFunc("GET /api/v1/files/{emulator}/{path...}", h.handleGetFile)
	mux.HandleFunc("PUT /api/v1/files/{emulator}/{path...}", h.handlePutFile)
	mux.HandleFunc("GET /api/v1/conflicts", h.handleListConflicts)
	mux.HandleFunc("POST /api/v1/conflicts/{id}/resolve", h.handleResolveConflict)
	mux.HandleFunc("GET /api/v1/history/{emulator}/{path...}", h.handleGetHistory)
	mux.HandleFunc("GET /api/v1/health", h.handleHealth)
}

func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handlers) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	emulator := r.PathValue("emulator")
	if err := ValidateName(emulator); err != nil {
		http.Error(w, "invalid emulator name", http.StatusBadRequest)
		return
	}

	manifest, err := h.storage.GetManifest(emulator)
	if err != nil {
		slog.Error("getting manifest", "emulator", emulator, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(manifest)
}

func (h *Handlers) handleGetFile(w http.ResponseWriter, r *http.Request) {
	emulator := r.PathValue("emulator")
	filePath := r.PathValue("path")
	if err := ValidateName(emulator); err != nil || filePath == "" {
		http.Error(w, "invalid emulator name or missing path", http.StatusBadRequest)
		return
	}

	reader, entry, err := h.storage.ReadFile(emulator, filePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-SHA256", entry.SHA256)
	w.Header().Set("X-Timestamp", entry.Timestamp.Format(time.RFC3339))
	w.Header().Set("X-Device-ID", entry.DeviceID)
	io.Copy(w, reader)
}

func (h *Handlers) handlePutFile(w http.ResponseWriter, r *http.Request) {
	emulator := r.PathValue("emulator")
	filePath := r.PathValue("path")
	if err := ValidateName(emulator); err != nil || filePath == "" {
		http.Error(w, "invalid emulator name or missing path", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	deviceID := r.Header.Get("X-Device-ID")
	sha256Hash := r.Header.Get("X-SHA256")
	timestampStr := r.Header.Get("X-Timestamp")
	baseHash := r.Header.Get("X-Base-Hash")

	if deviceID == "" || sha256Hash == "" || timestampStr == "" {
		http.Error(w, "X-Device-ID, X-SHA256, and X-Timestamp headers are required", http.StatusBadRequest)
		return
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		http.Error(w, "invalid X-Timestamp format (expected RFC3339)", http.StatusBadRequest)
		return
	}

	// Wrap body in a hashing reader to verify content hash and measure actual size
	h256 := sha256.New()
	countReader := &countingReader{r: io.TeeReader(r.Body, h256)}

	meta := model.FileEntry{
		SHA256:    sha256Hash,
		Size:      0, // will be set after write
		Timestamp: timestamp,
		DeviceID:  deviceID,
	}

	conflict, err := h.storage.WriteFile(emulator, filePath, countReader, meta, baseHash)
	if err != nil {
		slog.Error("writing file", "emulator", emulator, "path", filePath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Verify uploaded content hash matches declared hash
	computedHash := hex.EncodeToString(h256.Sum(nil))
	if computedHash != sha256Hash {
		slog.Warn("hash mismatch", "emulator", emulator, "path", filePath,
			"declared", sha256Hash, "computed", computedHash)
		http.Error(w, "hash mismatch: uploaded content does not match X-SHA256", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if conflict != nil {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "conflict",
			"conflict": conflict,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// countingReader wraps a reader and counts bytes read.
type countingReader struct {
	r io.Reader
	n int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	return n, err
}

func (h *Handlers) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	conflicts, err := h.storage.ListConflicts()
	if err != nil {
		slog.Error("listing conflicts", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"conflicts": conflicts})
}

func (h *Handlers) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ValidateName(id); err != nil {
		http.Error(w, "invalid conflict id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var body struct {
		Choice string `json:"choice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	body.Choice = strings.TrimSpace(body.Choice)
	if body.Choice != "local" && body.Choice != "remote" && body.Choice != "keep-both" {
		http.Error(w, "choice must be local, remote, or keep-both", http.StatusBadRequest)
		return
	}

	if err := h.storage.ResolveConflict(id, body.Choice); err != nil {
		slog.Error("resolving conflict", "id", id, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resolved"})
}

func (h *Handlers) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	emulator := r.PathValue("emulator")
	filePath := r.PathValue("path")
	if err := ValidateName(emulator); err != nil || filePath == "" {
		http.Error(w, "invalid emulator name or missing path", http.StatusBadRequest)
		return
	}

	versions, err := h.storage.GetHistory(emulator, filePath)
	if err != nil {
		slog.Error("getting history", "emulator", emulator, "path", filePath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"emulator": emulator,
		"path":     filePath,
		"versions": versions,
	})
}
