// Package database holds the Postgres pool and runs work in a transaction carried by the context, so
// repositories stay unaware of whether they run inside one.
package database

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is what repositories need; both the pool and a transaction provide it.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database url: %w", err)
	}
	// Sessions run in UTC whatever the server's or the caller's region: now() and every timestamptz read are UTC.
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	// A connection never goes back to the pool with a tenant still set (also after a panic in a request).
	cfg.AfterRelease = func(c *pgx.Conn) bool {
		_, err := c.Exec(context.Background(), `SELECT set_config('app.tenant_id', '', false)`)
		return err == nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// RequireRLS refuses a connection on which Row-Level Security would not apply: a superuser, a role with
// BYPASSRLS, or the owner of the tables. The service must never see another company's rows by configuration.
func (d *DB) RequireRLS(ctx context.Context) error {
	var who string
	var bypass bool
	err := d.Pool.QueryRow(ctx, `SELECT current_user, r.rolsuper OR r.rolbypassrls OR EXISTS (
		SELECT 1 FROM pg_tables t WHERE t.schemaname = 'pm' AND t.tableowner = current_user)
		FROM pg_roles r WHERE r.rolname = current_user`).Scan(&who, &bypass)
	if err != nil {
		return fmt.Errorf("check database role: %w", err)
	}
	if bypass {
		return fmt.Errorf("database role %q bypasses Row-Level Security (superuser, BYPASSRLS or table owner); connect as the application role alir_pm_app (see deploy/roles.sql)", who)
	}
	return nil
}

type txKey struct{}

type connKey struct{}

// Q returns the transaction of ctx when there is one, else the request's tenant connection, else the pool (no
// tenant: Row-Level Security answers no rows).
func (d *DB) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	if c, ok := ctx.Value(connKey{}).(*pgxpool.Conn); ok {
		return c
	}
	return d.Pool
}

// TenantScope gives the request one connection with app.tenant_id set to the caller's tenant, so Row-Level
// Security keeps every statement to that tenant. tenantOf reads the tenant from the request ("" = none).
func (d *DB) TenantScope(tenantOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tid := tenantOf(r)
			if tid == "" {
				next.ServeHTTP(w, r)
				return
			}
			c, err := d.Pool.Acquire(r.Context())
			if err != nil {
				http.Error(w, `{"code":"UNAVAILABLE","message":"Basis data tidak dapat dihubungi."}`, http.StatusServiceUnavailable)
				return
			}
			defer c.Release() // AfterRelease clears the tenant
			if _, err := c.Exec(r.Context(), `SELECT set_config('app.tenant_id', $1, false)`, tid); err != nil {
				http.Error(w, `{"code":"UNAVAILABLE","message":"Basis data tidak dapat dihubungi."}`, http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), connKey{}, c)))
		})
	}
}

// WithTenantConn puts a connection with the tenant already set in ctx (tests, background work).
func WithTenantConn(ctx context.Context, c *pgxpool.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, c)
}

// InTx runs fn in one transaction: committed when fn returns nil, rolled back otherwise. Nested calls join the outer one.
// A transaction Postgres aborts to break a deadlock or a serialization conflict is run again (fn must only touch
// the database and its own results), up to maxAttempts times, so racing saves are ordered instead of failing.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = d.once(ctx, fn); !retryable(err) {
			return err
		}
		time.Sleep(time.Duration(attempt*attempt*5+rand.IntN(10)) * time.Millisecond)
	}
	return err
}

const maxAttempts = 4

func (d *DB) once(ctx context.Context, fn func(ctx context.Context) error) error {
	var tx pgx.Tx
	var err error
	if c, ok := ctx.Value(connKey{}).(*pgxpool.Conn); ok {
		tx, err = c.Begin(ctx) // on the request's tenant connection
	} else {
		tx, err = d.Pool.Begin(ctx)
	}
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// retryable: deadlock_detected (40P01) or serialization_failure (40001).
func retryable(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (pg.Code == "40P01" || pg.Code == "40001")
}

// WithTx puts an existing transaction in ctx (tests roll it back afterwards).
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// Savepoint runs fn in a savepoint of the transaction in ctx (which must be in one): an error of fn — a refused row of
// an import, say — rolls back only what fn did, and the transaction goes on.
func (d *DB) Savepoint(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return errors.New("savepoint outside a transaction")
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(context.WithValue(ctx, txKey{}, sp)); err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	return sp.Commit(ctx)
}
