package main

import (
	"log"
	"net/http"
	"os"
)

const (
	defaultHost        = "127.0.0.1"
	defaultPort        = 9876
	defaultAPIPort     = 4321
	defaultMaxUploadMB = 10
)

func main() {
	var arg string
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}

	path, err := locateConfig(arg)
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		log.Fatal(err)
	}

	srv := newServer(cfg)
	if info, err := os.Stat(srv.workDir); err != nil {
		log.Fatalf("work_dir %s: %v", srv.workDir, err)
	} else if !info.IsDir() {
		log.Fatalf("work_dir %s: not a directory", srv.workDir)
	}
	if cfg.APIEnabled {
		if cfg.APIToken == "" {
			log.Fatal("api_enabled is true but api_token is empty")
		}
		apiAddr := cfg.apiListenAddr()
		log.Printf("subs api: listening on %s", apiAddr)
		go func() {
			if err := http.ListenAndServe(apiAddr, newAPIServer(cfg)); err != nil {
				log.Fatal(err)
			}
		}()
	}

	addr := cfg.listenAddr()
	log.Printf("subs: config=%s work_dir=%s listening on %s", path, srv.workDir, addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatal(err)
	}
}
