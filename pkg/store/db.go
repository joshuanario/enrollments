// Package store is the persistence layer: GORM row models and plain CRUD over
// Postgres or SQLite. It does not enforce any business rules.
package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

// Open connects to the database named by dsn and migrates the schema.
// The scheme picks the driver:
//
//	postgres://user:pass@host:5432/db?sslmode=disable   Postgres (the app, docker-compose.yml)
//	sqlite:file:name?mode=memory&cache=shared           SQLite, in memory (tests; see OpenInMemory)
//	sqlite:path/to/file.db                              SQLite, on disk
//
// Postgres connections are retried while the server is still starting.
func Open(ctx context.Context, dsn string) (*Store, error) {
	// Store methods already turn "record not found" into ErrNotFound, so it
	// isn't worth a log line.
	return open(ctx, dsn, logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	}))
}

func open(ctx context.Context, dsn string, l logger.Interface) (*Store, error) {
	cfg := &gorm.Config{
		Logger:  l,
		NowFunc: func() time.Time { return time.Now().UTC() },
		// Driver-neutral errors, e.g. a unique violation is gorm.ErrDuplicatedKey
		// on both Postgres and SQLite.
		TranslateError: true,
	}

	var (
		db  *gorm.DB
		err error
	)
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		db, err = openPostgres(ctx, dsn, cfg)
	case strings.HasPrefix(dsn, "sqlite:"):
		db, err = openSQLite(strings.TrimPrefix(dsn, "sqlite:"), cfg)
	default:
		return nil, fmt.Errorf("unsupported database url %q: want postgres:// or sqlite:", dsn)
	}
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.Migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

var memoryDBs atomic.Int64

// OpenInMemory opens a new, private in-memory SQLite database. Each call gets
// its own empty database, which disappears when the Store is closed. GORM's
// logging is off, since tests expect errors such as duplicate keys; the
// errors are still returned.
func OpenInMemory(ctx context.Context) (*Store, error) {
	name := fmt.Sprintf("enrollments-%d", memoryDBs.Add(1))
	return open(ctx, "sqlite:file:"+name+"?mode=memory&cache=shared", logger.Discard)
}

func openPostgres(ctx context.Context, dsn string, cfg *gorm.Config) (*gorm.DB, error) {
	var db *gorm.DB
	var err error
	backoff := 500 * time.Millisecond
	deadline := time.Now().Add(30 * time.Second)
	for {
		db, err = gorm.Open(postgres.Open(dsn), cfg)
		if err == nil {
			sqlDB, _ := db.DB()
			if err = sqlDB.PingContext(ctx); err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}
		log.Printf("postgres not ready (%v), retrying in %s", err, backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 4*time.Second {
			backoff *= 2
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

func openSQLite(dsn string, cfg *gorm.Config) (*gorm.DB, error) {
	// Enforce foreign keys like Postgres does (SQLite has them off by default).
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := gorm.Open(sqlite.Open(dsn+sep+"_pragma=foreign_keys(1)"), cfg)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer at a time; a single connection avoids
	// "database is locked" errors and keeps an in-memory database alive
	// for the life of the Store.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	return db, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&PathRow{}, &LessonRow{}, &EnrollmentRow{}, &EventRow{})
}

// Tx runs fn inside a single database transaction. The *Store passed to fn is
// bound to that transaction; returning an error rolls it back. Inside fn, use
// only the tx store: on SQLite (one connection) calling the outer store would
// wait forever for the connection fn is holding.
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

// DB exposes the underlying handle for queries the repo doesn't cover.
func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
