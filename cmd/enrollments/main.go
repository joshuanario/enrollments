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
	srv := setupServer(ctx, db)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
	listenForShutdown(ctx, wait, srv)
}

func setupServer(ctx context.Context, db *store.Store) *http.Server {

	log.Println("enrollments<-0.0.0.0:8080")
	return &http.Server{
		Addr:         "0.0.0.0:8080",
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      NewRouter(db),
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

func NewRouter(db *store.Store) http.Handler {
	r := mux.NewRouter()

	// TODO(you): JSON API routes go here, e.g. r.PathPrefix("/api").Subrouter().

	// TODO(you): replace NotImplemented with your service (built on db).
	_ = db
	web.Register(r, operations.OperationsService{})

	// foldersRoute := r.PathPrefix("/folders").Subrouter()
	// foldersRoute.HandleFunc("/{id}", Catalog()).Methods("GET")
	// projectIDRoute := foldersRoute.PathPrefix("/{id}").Subrouter()
	// projectIDRoute.HandleFunc("/facts", Expressable()).Methods("GET")
	// projectIDRoute.HandleFunc("/{hash}", Renderable()).Methods("GET")
	// wd, err := os.Getwd()
	// if err != nil {
	// 	panic(err)
	// }
	// conceptnetworkbrowser := http.FileServer(http.Dir((filepath.Join(wd, "goldlord-midas"))))
	// r.PathPrefix("/").Handler(http.StripPrefix("/", conceptnetworkbrowser))
	return r
}
