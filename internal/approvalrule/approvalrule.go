// Package approvalrule is the Portal Master master "Aturan Approval" (menu pm.approvalRule): per document type, how
// far a document climbs the reporting line — a number of superiors, up to the head of a level, or by amount.
package approvalrule

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// Band: amounts up to Max (rupiah; nil = no upper bound) climb up to the head of Level.
type Band struct {
	Max     *float64 `json:"max"`
	Level   string   `json:"level"`
	levelID string
}

// Reach is how far a document climbs (the shape ALIR's ApprovalReach has).
type Reach struct {
	Kind    string `json:"kind"` // steps | level | amount
	Steps   *int   `json:"steps,omitempty"`
	Level   string `json:"level,omitempty"`
	Bands   []Band `json:"bands,omitempty"`
	levelID *string
}

type Rule struct {
	record.Base
	Sop      string `json:"sop"`
	Module   string `json:"module"`
	Reach    Reach  `json:"reach"`
	MinSteps int    `json:"minSteps"`
}

type Input struct {
	record.Input
	Sop, Module string
	Reach       Reach
	MinSteps    int
}

const maxSop, maxModule = 60, 60

var kinds = []string{"steps", "level", "amount"}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Sop = record.RequiredText(v, "sop", "Dasar SOP", maxSop, &p)
	in.Module = record.RequiredText(v, "module", "Modul", maxModule, &p)
	in.MinSteps = record.RequiredInt(v, "minSteps", "Minimal penyetuju", 0, &p)
	in.Reach = parseReach(v["reach"], &p)
	return in, p
}

// parseReach reads the nested reach object; its problems are reported as reach.<field>.
func parseReach(x any, p *record.Problems) Reach {
	m, ok := x.(map[string]any)
	if !ok {
		p.Add("reach.kind", "Jangkauan wajib diisi.")
		return Reach{}
	}
	var q record.Problems
	defer func() {
		for _, pr := range q {
			p.Add("reach."+pr.Field, pr.Message)
		}
	}()
	r := Reach{Kind: record.Choice(m, "kind", "Jangkauan", kinds, &q)}
	switch r.Kind {
	case "steps":
		n := record.RequiredInt(m, "steps", "Jumlah atasan", 1, &q)
		r.Steps = &n
	case "level":
		r.Level = record.RequiredText(m, "level", "Sampai level", record.MaxCode, &q)
	case "amount":
		list, ok := m["bands"].([]any)
		if !ok || len(list) == 0 {
			p.Add("reach.bands", "Pita nilai wajib diisi: satu baris per pita.")
			return r
		}
		if len(list) > 20 {
			p.Add("reach.bands", "Pita nilai paling banyak 20.")
			return r
		}
		for i, it := range list {
			b, ok := it.(map[string]any)
			level, _ := b["level"].(string)
			if !ok || strings.TrimSpace(level) == "" || !record.Clean(level) {
				p.Add("reach.bands", fmt.Sprintf("Pita ke-%d: level wajib diisi.", i+1))
				return r
			}
			band := Band{Level: strings.TrimSpace(level)}
			if b["max"] != nil {
				f, ok := amount(b["max"])
				if !ok || f <= 0 || f >= 1e16 {
					p.Add("reach.bands", fmt.Sprintf("Pita ke-%d: nilai maksimal harus angka lebih dari 0.", i+1))
					return r
				}
				band.Max = &f
			}
			r.Bands = append(r.Bands, band)
		}
		for i, b := range r.Bands {
			switch {
			case b.Max == nil && i != len(r.Bands)-1:
				p.Add("reach.bands", "Pita tanpa nilai maksimal (di atasnya) hanya boleh di baris terakhir.")
				return r
			case i > 0 && b.Max != nil && *b.Max <= *r.Bands[i-1].Max:
				p.Add("reach.bands", "Nilai maksimal pita harus naik dari baris ke baris.")
				return r
			}
		}
	}
	return r
}

func amount(x any) (float64, bool) {
	switch n := x.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
	case float64:
		return n, true
	}
	return 0, false
}

func values(x Rule) record.Values {
	v := record.ValuesOf(x.Base)
	v["sop"], v["module"], v["minSteps"] = x.Sop, x.Module, x.MinSteps
	reach := map[string]any{"kind": x.Reach.Kind}
	switch x.Reach.Kind {
	case "steps":
		reach["steps"] = *x.Reach.Steps
	case "level":
		reach["level"] = x.Reach.Level
	case "amount":
		var bands []any
		for _, b := range x.Reach.Bands {
			m := map[string]any{"level": b.Level, "max": nil}
			if b.Max != nil {
				m["max"] = *b.Max
			}
			bands = append(bands, m)
		}
		reach["bands"] = bands
	}
	v["reach"] = reach
	return v
}

func New(db *database.DB) *master.Service[Rule, Input] {
	return &master.Service[Rule, Input]{
		Spec: master.Spec[Rule, Input]{
			Menu: "pm.approvalRule", Table: "approval_rule", Noun: "Aturan approval",
			Base:    func(x *Rule) *record.Base { return &x.Base },
			Values:  values,
			Parse:   Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Rule, p record.Problems) (record.Problems, error) {
				switch in.Reach.Kind {
				case "level":
					var kept *string
					if stored != nil {
						kept = stored.Reach.levelID
					}
					l, err := refs.Pick(ctx, db, "org_level", "reach.level", "Level", nonEmpty(in.Reach.Level), kept, &p)
					if err != nil || l == nil {
						return p, err
					}
					in.Reach.levelID, in.Reach.Level = &l.ID, l.Code
				case "amount":
					prevRank := math.MaxInt
					for i := range in.Reach.Bands {
						b := &in.Reach.Bands[i]
						l, err := refs.Pick(ctx, db, "org_level", "reach.bands", "Level", &b.Level, nil, &p)
						if err != nil || l == nil {
							return p, err
						}
						b.levelID, b.Level = l.ID, l.Code
						var rank int
						if err := db.Q(ctx).QueryRow(ctx, `SELECT rank FROM pm.org_level WHERE id = $1`, l.ID).Scan(&rank); err != nil {
							return p, err
						}
						if rank > prevRank {
							p.Add("reach.bands", "Pita dengan nilai lebih besar harus sampai level yang sama atau lebih tinggi.")
							return p, nil
						}
						prevRank = rank
					}
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type repo struct{ db *database.DB }

var selectRule = `SELECT ` + record.ColumnsAs("r") + `, r.sop, r.module, r.reach_kind, r.reach_steps, r.reach_level_id, l.code, r.min_steps
	FROM pm.approval_rule r LEFT JOIN pm.org_level l ON l.id = r.reach_level_id`

func scan(row pgx.Row) (Rule, error) {
	var x Rule
	var level *string
	err := x.Scan(row, &x.Sop, &x.Module, &x.Reach.Kind, &x.Reach.Steps, &x.Reach.levelID, &level, &x.MinSteps)
	if level != nil {
		x.Reach.Level = *level
	}
	return x, err
}

// withBands fills the amount bands of rules in one query.
func (r repo) withBands(ctx context.Context, rules []Rule) ([]Rule, error) {
	ids := []string{}
	at := map[string]int{}
	for i, x := range rules {
		if x.Reach.Kind == "amount" {
			ids = append(ids, x.ID)
			at[x.ID] = i
		}
	}
	if len(ids) == 0 {
		return rules, nil
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT b.approval_rule_id, b.max_amount::float8, l.code, b.org_level_id FROM pm.approval_rule_band b
		JOIN pm.org_level l ON l.id = b.org_level_id WHERE b.approval_rule_id = ANY($1::uuid[]) ORDER BY b.approval_rule_id, b.seq`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var b Band
		if err := rows.Scan(&id, &b.Max, &b.Level, &b.levelID); err != nil {
			return nil, err
		}
		rules[at[id]].Reach.Bands = append(rules[at[id]].Reach.Bands, b)
	}
	return rules, rows.Err()
}

func (r repo) one(ctx context.Context, q string, args ...any) (Rule, error) {
	x, err := master.One(ctx, r.db, scan, q, args...)
	if err != nil {
		return x, err
	}
	out, err := r.withBands(ctx, []Rule{x})
	if err != nil {
		return x, err
	}
	return out[0], nil
}

func (r repo) List(ctx context.Context) ([]Rule, error) {
	list, err := master.Rows(ctx, r.db, scan, selectRule+` WHERE NOT r.is_deleted ORDER BY r.code`)
	if err != nil {
		return nil, err
	}
	return r.withBands(ctx, list)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Rule, error) {
	q := selectRule + ` WHERE lower(r.code) = lower($1) AND NOT r.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF r`
	}
	return r.one(ctx, q, code)
}

// reachColumns are the reach_steps and reach_level_id of a reach (null when its kind does not use them).
func reachColumns(x Reach) (*int, *string) {
	switch x.Kind {
	case "steps":
		return x.Steps, nil
	case "level":
		return nil, x.levelID
	}
	return nil, nil
}

func (r repo) saveBands(ctx context.Context, id string, x Reach) error {
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM pm.approval_rule_band WHERE approval_rule_id = $1`, id); err != nil {
		return err
	}
	if x.Kind != "amount" {
		return nil
	}
	for i, b := range x.Bands {
		if _, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO pm.approval_rule_band (approval_rule_id, seq, max_amount, org_level_id) VALUES ($1, $2, $3, $4)`,
			id, i+1, b.Max, b.levelID); err != nil {
			return err
		}
	}
	return nil
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Rule, error) {
	steps, level := reachColumns(in.Reach)
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.approval_rule (code, name, description, is_active, valid_from, valid_to, created_by,
		sop, module, reach_kind, reach_steps, reach_level_id, min_steps)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Sop, in.Module, in.Reach.Kind, steps, level, in.MinSteps).Scan(&id); err != nil {
		return Rule{}, err
	}
	if err := r.saveBands(ctx, id, in.Reach); err != nil {
		return Rule{}, err
	}
	return r.one(ctx, selectRule+` WHERE r.id = $1`, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Rule, error) {
	steps, level := reachColumns(in.Reach)
	if err := master.Exec(ctx, r.db, `UPDATE pm.approval_rule SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, sop = $7, module = $8, reach_kind = $9, reach_steps = $10, reach_level_id = $11, min_steps = $12,
		updated_at = now(), updated_by = $13, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Sop, in.Module, in.Reach.Kind, steps, level, in.MinSteps, actor); err != nil {
		return Rule{}, err
	}
	if err := r.saveBands(ctx, id, in.Reach); err != nil {
		return Rule{}, err
	}
	return r.one(ctx, selectRule+` WHERE r.id = $1`, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "approval_rule", id, actor)
}
