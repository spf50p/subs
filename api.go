package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// multipartMemoryBytes is how much of the upload is buffered in memory before
// spilling to a temp file; the hard size cap comes from http.MaxBytesReader.
const multipartMemoryBytes = 1 << 20 // 1 MiB

// apiServer handles the authenticated management API.
type apiServer struct {
	cfg     *Config
	workDir string
}

func newAPIServer(cfg *Config) http.Handler {
	s := &apiServer{cfg: cfg, workDir: cfg.resolvedWorkDir()}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/create", s.handleCreate)
	mux.HandleFunc("/api/update", s.handleUpdate)
	mux.HandleFunc("/api/delete", s.handleDelete)
	mux.HandleFunc("/api/check", s.handleCheck)
	return mux
}

// handleCreate creates an empty subscription: work_dir/<subid>/ with an empty
// subs.yaml. Peers are added later via /api/update. An optional id query
// parameter reuses a specific subid; otherwise one is generated. Returns the id.
func (s *apiServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeFail(w, http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeFail(w, http.StatusUnauthorized)
		return
	}

	subid := strings.TrimSpace(r.URL.Query().Get("id"))
	if subid == "" {
		var err error
		subid, err = randomHex(16)
		if err != nil {
			log.Printf("generate subid: %v", err)
			writeFail(w, http.StatusInternalServerError)
			return
		}
	} else if !validID(subid) {
		writeFail(w, http.StatusBadRequest)
		return
	}

	dir := filepath.Join(s.workDir, subid)
	peersPath := filepath.Join(dir, peersFile)
	if _, err := os.Stat(peersPath); err == nil {
		// Subscription already exists; refuse to clobber it.
		writeFail(w, http.StatusConflict)
		return
	} else if !os.IsNotExist(err) {
		log.Printf("stat %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("create dir %s: %v", dir, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(peersPath, nil, 0o644); err != nil {
		log.Printf("create %s: %v", peersPath, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	writeSuccess(w, http.StatusCreated, map[string]any{"id": subid})
}

// handleUpdate appends a new peer to an existing subscription identified by the
// id query parameter. It accepts the same multipart form as handleCreate (minus
// subid) and responds with {"success": true} or {"success": false}.
func (s *apiServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeFail(w, http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeFail(w, http.StatusUnauthorized)
		return
	}

	subid := strings.TrimSpace(r.URL.Query().Get("id"))
	if subid == "" || !validID(subid) {
		writeFail(w, http.StatusBadRequest)
		return
	}

	dir := filepath.Join(s.workDir, subid)
	peersPath := filepath.Join(dir, peersFile)
	if _, err := loadClient(peersPath); err != nil {
		if os.IsNotExist(err) {
			writeFail(w, http.StatusNotFound)
			return
		}
		log.Printf("update %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	upload, ok := s.parsePeerUpload(w, r)
	if !ok {
		return
	}
	defer upload.file.Close()

	// exclusive=true replaces the peer list with just this peer instead of
	// appending; default (omitted/false) keeps the previous append behaviour.
	exclusive := false
	if v := strings.TrimSpace(r.FormValue("exclusive")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeFail(w, http.StatusBadRequest)
			return
		}
		exclusive = b
	}

	if err := saveUpload(filepath.Join(dir, upload.fileName), upload.file); err != nil {
		log.Printf("save upload for %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	var err error
	if exclusive {
		err = writeClient(peersPath, Client{upload.peer})
	} else {
		err = appendPeer(peersPath, upload.peer)
	}
	if err != nil {
		log.Printf("write peer for %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

// handleDelete removes the subscription directory (and all of its contents) for
// the id query parameter. Responds with {"success": true} or {"success": false}
// when no such subscription exists.
func (s *apiServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeFail(w, http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeFail(w, http.StatusUnauthorized)
		return
	}

	subid := strings.TrimSpace(r.URL.Query().Get("id"))
	if subid == "" || !validID(subid) {
		writeFail(w, http.StatusBadRequest)
		return
	}

	dir := filepath.Join(s.workDir, subid)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			writeFail(w, http.StatusNotFound)
			return
		}
		log.Printf("stat %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	if err := os.RemoveAll(dir); err != nil {
		log.Printf("delete %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

// peerUpload is the parsed multipart form shared by the create and update
// endpoints. The caller must close file once done with it.
type peerUpload struct {
	fileName string
	file     multipart.File
	peer     Peer
}

// parsePeerUpload parses and validates the multipart form (title and
// config_file required, comment/link optional). On failure it writes the
// {"success": false} response and returns ok=false.
func (s *apiServer) parsePeerUpload(w http.ResponseWriter, r *http.Request) (peerUpload, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.maxUploadBytes())
	if err := r.ParseMultipartForm(multipartMemoryBytes); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeFail(w, http.StatusRequestEntityTooLarge)
			return peerUpload{}, false
		}
		writeFail(w, http.StatusBadRequest)
		return peerUpload{}, false
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		writeFail(w, http.StatusBadRequest)
		return peerUpload{}, false
	}

	file, header, err := r.FormFile("config_file")
	if err != nil {
		writeFail(w, http.StatusBadRequest)
		return peerUpload{}, false
	}

	fileName := filepath.Base(header.Filename)
	if fileName == "." || fileName == string(filepath.Separator) || fileName == "" {
		file.Close()
		writeFail(w, http.StatusBadRequest)
		return peerUpload{}, false
	}

	// qr is optional; when omitted the peer offers a QR code by default.
	var qr *bool
	if v := strings.TrimSpace(r.FormValue("qr")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			file.Close()
			writeFail(w, http.StatusBadRequest)
			return peerUpload{}, false
		}
		qr = &b
	}

	return peerUpload{
		fileName: fileName,
		file:     file,
		peer: Peer{
			Title:      title,
			Comment:    strings.TrimSpace(r.FormValue("comment")),
			Link:       strings.TrimSpace(r.FormValue("link")),
			ConfigFile: fileName,
			QR:         qr,
		},
	}, true
}

// handleCheck reports whether a subscription exists for the given subid.
func (s *apiServer) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeFail(w, http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeFail(w, http.StatusUnauthorized)
		return
	}

	subid := strings.TrimSpace(r.URL.Query().Get("subid"))
	if subid == "" || !validID(subid) {
		writeFail(w, http.StatusBadRequest)
		return
	}

	_, err := loadClient(filepath.Join(s.workDir, subid, peersFile))
	if os.IsNotExist(err) {
		writeSuccess(w, http.StatusOK, map[string]any{"exists": false})
		return
	}
	if err != nil {
		log.Printf("check %q: %v", subid, err)
		writeFail(w, http.StatusInternalServerError)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"exists": true})
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeSuccess writes a {"success": true, ...extra} JSON response.
func writeSuccess(w http.ResponseWriter, status int, extra map[string]any) {
	m := map[string]any{"success": true}
	maps.Copy(m, extra)
	writeJSON(w, status, m)
}

// writeFail writes a {"success": false} JSON response with the given status code.
func writeFail(w http.ResponseWriter, status int) {
	writeJSON(w, status, map[string]any{"success": false})
}

// authorized reports whether the request carries the configured bearer token.
func (s *apiServer) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	token := strings.TrimSpace(h[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.APIToken)) == 1
}

// saveUpload writes the contents of src to path.
func saveUpload(path string, src io.Reader) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, src); err != nil {
		return err
	}
	return nil
}

// randomHex returns a hex string of n random bytes (2*n characters), matching
// `openssl rand -hex n`.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
