package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"github.com/joshuanario/enrollments/pkg/operations"
	"github.com/joshuanario/enrollments/pkg/store"
	"github.com/joshuanario/enrollments/pkg/web"
)

func main() {
	log.Println("Work by Joshua Nario.  See ASSIGNMENT.MD")
	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	var sortingFlag int
	flag.IntVar(&sortingFlag, "sorting", 0, "the sorting method for lessons (0 = fractional-indexing, 1 = integer-position, 2 = adjacency-list)")
	flag.Parse()
	var ctx = context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required, e.g. postgres://enrollments:enrollments@localhost:5432/enrollments?sslmode=disable")
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	log.Println("connected to postgres, schema migrated")
	ss := operations.SortingStrategy(sortingFlag)
	switch ss {
	case operations.FractionalIndexing, operations.IntegerPositionColumn, operations.AdjacencyList:
	default:
		panic("invalid sorting method")
	}
	if os.Getenv("SEED_SETUP") == "true" {
		fixtures := os.Getenv("FIXTURES_DIR")
		if fixtures == "" {
			fixtures = "test/fixtures"
		}
		if err := seedSetupPaths(ctx, db, operations.NewOperationsService(db, &ss), fixtures); err != nil {
			log.Fatal(err)
		}
	}
	srv := setupServer(db, &ss)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
	listenForShutdown(ctx, wait, srv)
}

func setupServer(db *store.Store, ss *operations.SortingStrategy) *http.Server {

	log.Println("enrollments is running on http://localhost:8080")
	return &http.Server{
		Addr:         "0.0.0.0:8080",
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      NewRouter(db, ss),
	}
}

func listenForShutdown(ctx context.Context, grace time.Duration, srv *http.Server) {
	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)
	<-c
	log.Println("Shutting down")
	ctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	srv.Shutdown(ctx)
	os.Exit(0)
}

func NewRouter(db *store.Store, ss *operations.SortingStrategy) http.Handler {
	r := mux.NewRouter()
	svc := operations.NewOperationsService(db, ss)
	web.Register(r, svc)

	return r
}
