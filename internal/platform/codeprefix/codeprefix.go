// Package codeprefix generates the code of a new master record from its row in pm.code_prefix (the setup data of
// the future master "Prefix Kode Master"): prefix + the next number, left-padded with zeros, e.g. ML001.
package codeprefix

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/database"
)

// Postgres hands out numbers from pm.code_prefix.
type Postgres struct{ DB *database.DB }

// Next takes the next number of the master table and returns its code. Call it inside the transaction that saves
// the record: the counter row stays locked until it ends, so two saves never get one number, and a save that
// fails gives its number back.
func (p Postgres) Next(ctx context.Context, master string) (string, error) {
	var prefix string
	var digits int
	var n int64
	err := p.DB.Q(ctx).QueryRow(ctx,
		`UPDATE pm.code_prefix SET last_number = last_number + 1
		 WHERE lower(code) = lower($1) AND is_active AND NOT is_deleted
		 RETURNING prefix, digits, last_number`, master).Scan(&prefix, &digits, &n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", NotSetUp(master)
	}
	if err != nil {
		return "", err
	}
	return Format(prefix, digits, n), nil
}

// Format is prefix followed by n, left-padded with zeros to digits (a longer n is kept whole).
func Format(prefix string, digits int, n int64) string {
	return fmt.Sprintf("%s%0*d", prefix, digits, n)
}

// NotSetUp is the answer when a master has no active prefix.
func NotSetUp(master string) error {
	return apperr.Conflict("NO_PREFIX", fmt.Sprintf("Prefix kode untuk %s belum disiapkan atau nonaktif. Hubungi admin Portal Master.", master))
}

// Generator hands out codes (Postgres in the service, a counter in tests).
type Generator interface {
	Next(ctx context.Context, master string) (string, error)
}

// maxTries bounds the numbers skipped because a code was already taken (entered before codes were generated, or a
// counter set back).
const maxTries = 20

// NextFree is the first generated code of master that taken says no row holds (deleted rows included).
func NextFree(ctx context.Context, gen Generator, master string, taken func(ctx context.Context, code string) (bool, error)) (string, error) {
	for range maxTries {
		code, err := gen.Next(ctx, master)
		if err != nil {
			return "", err
		}
		used, err := taken(ctx, code)
		if err != nil {
			return "", err
		}
		if !used {
			return code, nil
		}
	}
	return "", apperr.Conflict("CODE_EXHAUSTED", "Kode otomatis berikutnya sudah dipakai. Periksa nomor terakhir di Prefix Kode Master.")
}

// Global numbers the records of a global master (one set for every tenant, so no tenant's pm.code_prefix row
// applies): its fixed prefix + one past the highest number in use. The table's transaction lock makes two saves
// take one number after the other.
type Global struct {
	DB     *database.DB
	Prefix string
	Digits int
}

func (g Global) Next(ctx context.Context, master string) (string, error) {
	if _, err := g.DB.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('pm.global.' || $1))`, master); err != nil {
		return "", err
	}
	var n int64
	if err := g.DB.Q(ctx).QueryRow(ctx, `SELECT coalesce(max(substring(code FROM $2::int + 1)::bigint), 0) + 1 FROM pm.`+master+
		` WHERE code ~ ('^' || $1 || '[0-9]+$')`, g.Prefix, len(g.Prefix)).Scan(&n); err != nil {
		return "", err
	}
	return Format(g.Prefix, g.Digits, n), nil
}
