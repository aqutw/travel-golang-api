package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/httpapi"
	"github.com/aqutw/travel-golang-api/internal/workspace"
	"github.com/gin-gonic/gin"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	now := time.Now()
	if raw := os.Getenv("REFERENCE_DATE"); raw != "" {
		var err error
		now, err = time.ParseInLocation("2006-01-02", raw, time.FixedZone("Asia/Taipei", 8*60*60))
		if err != nil {
			return errors.New("REFERENCE_DATE must use YYYY-MM-DD")
		}
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/travel.sqlite3"
	}
	c, err := catalog.New(dbPath, now)
	if err != nil {
		return err
	}
	defer c.Close()
	apiKey := os.Getenv("API_WRITE_KEY")
	if apiKey != "" && len(apiKey) < 32 {
		return errors.New("API_WRITE_KEY must contain at least 32 bytes")
	}
	origins := []string{"*"}
	if raw := os.Getenv("CORS_ORIGINS"); raw != "" {
		origins = strings.Split(raw, ",")
		for i, origin := range origins {
			origin = strings.TrimSpace(origin)
			origins[i] = origin
			if origin == "*" {
				if len(origins) != 1 {
					return errors.New("CORS_ORIGINS wildcard must be used alone")
				}
				continue
			}
			u, err := url.Parse(origin)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
				return fmt.Errorf("invalid CORS_ORIGINS origin: %q", origin)
			}
		}
	}
	var store *workspace.Store
	if apiKey != "" {
		store, err = workspace.New(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
	}
	gin.SetMode(gin.ReleaseMode)
	srv := &http.Server{Addr: addr, Handler: httpapi.New(c, httpapi.Options{Store: store, APIKey: apiKey, AllowedOrigins: origins, Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil))}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	go func() {
		log.Printf("travel mock API listening on %s (reference_date=%s)", addr, c.Today.Format("2006-01-02"))
		result <- srv.ListenAndServe()
	}()
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(timeout)
	}
	return nil
}
