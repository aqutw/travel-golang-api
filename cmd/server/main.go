package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aqutw/travel-golang-api/internal/catalog"
	"github.com/aqutw/travel-golang-api/internal/httpapi"
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
	srv := &http.Server{Addr: addr, Handler: httpapi.New(c), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
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
