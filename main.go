package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"shipdude/internal/db"
	"shipdude/internal/factory"
	apphttp "shipdude/internal/http"
	"shipdude/pkg/config"
	"shipdude/pkg/util"
)

//go:embed web/*
var staticFS embed.FS

func main() {
	// Load optional .env overrides before reading any configuration.
	_ = util.LoadDotEnv(".env")

	staticHandler, err := staticHandler()
	if err != nil {
		log.Fatalf("failed to locate embedded static assets sub-directory: %v", err)
	}

	database, err := db.Connect(config.DBPath())
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer database.Close()

	f := factory.NewFactory(database)
	router := apphttp.NewRouter(f, staticHandler)

	address := ":" + config.AppPort()
	log.Printf("=============================================")
	log.Printf("  %s Web Portal Starting...             ", config.AppName())
	log.Printf("  Access local portal at http://localhost%s  ", address)
	log.Printf("=============================================")

	if err := http.ListenAndServe(address, router); err != nil {
		log.Fatalf("failed to bind server to port %s: %v", address, err)
	}
}

// staticHandler returns the embedded web directory as an HTTP handler.
func staticHandler() (http.Handler, error) {
	subFS, err := fs.Sub(staticFS, "web")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(subFS)), nil
}
