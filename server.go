package main

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

const peersFile = "subs.yaml"

// server holds the runtime state for handling requests.
type server struct {
	cfg     *Config
	workDir string
}

func newServer(cfg *Config) *server {
	return &server{cfg: cfg, workDir: cfg.resolvedWorkDir()}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	id := parts[0]
	if id == "" || !validID(id) || len(parts) > 2 {
		http.NotFound(w, r)
		return
	}

	dir := filepath.Join(s.workDir, id)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		http.NotFound(w, r)
		return
	}

	client, err := loadClient(filepath.Join(dir, peersFile))
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("load client for %q: %v", id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if len(parts) == 2 {
		s.serveConfig(w, r, dir, parts[1], client)
		return
	}
	s.renderPage(w, id, client)
}

// renderPage writes the subscription HTML page for the given subid.
func (s *server) renderPage(w http.ResponseWriter, id string, client *Client) {
	var buf bytes.Buffer
	data := pageData{Title: s.cfg.Title, Description: s.cfg.Description, Subid: id, Peers: *client}
	if err := pageTemplate.Execute(&buf, data); err != nil {
		log.Printf("render %q: %v", id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Default content type; may be overridden by config headers.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for k, v := range s.cfg.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// serveConfig serves a peer's config file. Only files declared as a config_file
// in subs.yaml are served. With the "qr" query parameter it returns a PNG QR
// code of the file contents; otherwise the file is sent as a download.
func (s *server) serveConfig(w http.ResponseWriter, r *http.Request, dir, name string, client *Client) {
	if !validID(name) || !client.hasConfigFile(name) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(dir, name)
	if r.URL.Query().Has("qr") {
		if !client.qrAllowed(name) {
			http.NotFound(w, r)
			return
		}
		s.serveQR(w, r, full)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, full)
}

// serveQR returns a PNG QR code encoding the contents of the file at path.
func (s *server) serveQR(w http.ResponseWriter, r *http.Request, path string) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("read config for qr %q: %v", path, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	png, err := qrcode.Encode(string(data), qrcode.Medium, 512)
	if err != nil {
		log.Printf("qr encode %q: %v", path, err)
		http.Error(w, "cannot generate qr", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

// validID rejects path segments that could escape the work directory or
// reference nested paths.
func validID(id string) bool {
	if id == "." || id == ".." {
		return false
	}
	return !strings.ContainsAny(id, "/\\") && !strings.Contains(id, "..")
}
