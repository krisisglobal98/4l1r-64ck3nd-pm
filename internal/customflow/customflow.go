// Package customflow is the Portal Master master "Approval Custom" (menu pm.approvalFlow): a custom chain of
// approvers that overrides the default reporting line for one document type inside one branch of the organization
// (the unit and every unit under it). The most specific branch wins, so one flow per document type and branch.
package customflow

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type Flow struct {
	record.Base
	Sop           string   `json:"sop"`
	RuleID        string   `json:"ruleId"`
	RuleCode      string   `json:"ruleCode"`
	ScopeUnitID   string   `json:"scopeUnitId"`
	ScopeUnitCode string   `json:"scopeUnitCode"`
	StepCodes     []string `json:"stepCodes"` // positions that sign, in order
	stepIDs       []string
}

type Input struct {
	record.Input
	Sop, RuleCode, ScopeUnitCode string
	StepCodes                    []string
	RuleID, ScopeUnitID          string
	StepIDs                      []string
}

const maxSop, maxSteps = 60, 20

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Sop = record.RequiredText(v, "sop", "Dasar SOP", maxSop, &p)
	in.RuleCode = record.RequiredText(v, "ruleCode", "Jenis dokumen", record.MaxCode, &p)
	in.ScopeUnitCode = record.RequiredText(v, "scopeUnitCode", "Cakupan unit", record.MaxCode, &p)
	in.StepCodes = record.Codes(v, "stepCodes", "Langkah penyetuju", &p)
	switch {
	case p.Has("stepCodes"):
	case len(in.StepCodes) == 0:
		p.Add("stepCodes", "Langkah penyetuju wajib diisi: pilih minimal satu posisi.")
	case len(in.StepCodes) > maxSteps:
		p.Add("stepCodes", fmt.Sprintf("Langkah penyetuju paling banyak %d.", maxSteps))
	}
	return in, p
}

func values(f Flow) record.Values {
	v := record.ValuesOf(f.Base)
	v["sop"], v["ruleCode"], v["scopeUnitCode"] = f.Sop, f.RuleCode, f.ScopeUnitCode
	steps := []any{}
	for _, c := range f.StepCodes {
		steps = append(steps, c)
	}
	v["stepCodes"] = steps
	return v
}

func New(db *database.DB) *master.Service[Flow, Input] {
	return &master.Service[Flow, Input]{
		Spec: master.Spec[Flow, Input]{
			Menu: "pm.approvalFlow", Table: "custom_flow", Noun: "Approval custom",
			Base:    func(f *Flow) *record.Base { return &f.Base },
			Values:  values,
			Parse:   Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Flow, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Flow, p record.Problems) (record.Problems, error) {
	var keptRule, keptUnit *string
	if stored != nil {
		keptRule, keptUnit = &stored.RuleID, &stored.ScopeUnitID
	}
	rule, err := refs.Pick(ctx, db, "approval_rule", "ruleCode", "Jenis dokumen", nonEmpty(in.RuleCode), keptRule, &p)
	if err != nil {
		return p, err
	}
	unit, err := refs.Pick(ctx, db, "org_unit", "scopeUnitCode", "Unit", nonEmpty(in.ScopeUnitCode), keptUnit, &p)
	if err != nil {
		return p, err
	}
	for _, c := range in.StepCodes {
		var kept *string
		if r, err := refs.Lookup(ctx, db, "position", c); err == nil && stored != nil && slices.Contains(stored.stepIDs, r.ID) {
			kept = &r.ID
		}
		pos, err := refs.Pick(ctx, db, "position", "stepCodes", "Posisi", &c, kept, &p)
		if err != nil {
			return p, err
		}
		if pos != nil {
			in.StepIDs = append(in.StepIDs, pos.ID)
		}
	}
	if rule == nil || unit == nil {
		return p, nil
	}
	in.RuleID, in.ScopeUnitID = rule.ID, unit.ID
	self := ""
	if stored != nil {
		self = stored.ID
	}
	var other string
	err = db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.custom_flow WHERE approval_rule_id = $1 AND scope_unit_id = $2 AND NOT is_deleted AND id::text <> $3`,
		rule.ID, unit.ID, self).Scan(&other)
	switch {
	case err == nil:
		p.Add("scopeUnitCode", fmt.Sprintf("Sudah ada approval custom %s untuk %s di cakupan %s; ubah yang itu.", other, rule.Name, unit.Name))
	case !errors.Is(err, pgx.ErrNoRows):
		return p, err
	}
	return p, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type repo struct{ db *database.DB }

var selectFlow = `SELECT ` + record.ColumnsAs("f") + `, f.sop, f.approval_rule_id, r.code, f.scope_unit_id, u.code,
	coalesce(array_agg(x.code ORDER BY s.seq) FILTER (WHERE x.id IS NOT NULL), '{}'),
	coalesce(array_agg(x.id::text ORDER BY s.seq) FILTER (WHERE x.id IS NOT NULL), '{}')
	FROM pm.custom_flow f JOIN pm.approval_rule r ON r.id = f.approval_rule_id JOIN pm.org_unit u ON u.id = f.scope_unit_id
	LEFT JOIN pm.custom_flow_step s ON s.custom_flow_id = f.id LEFT JOIN pm.position x ON x.id = s.position_id`

const groupBy = ` GROUP BY f.id, r.code, u.code`

func scan(row pgx.Row) (Flow, error) {
	var f Flow
	return f, f.Scan(row, &f.Sop, &f.RuleID, &f.RuleCode, &f.ScopeUnitID, &f.ScopeUnitCode, &f.StepCodes, &f.stepIDs)
}

func (r repo) List(ctx context.Context) ([]Flow, error) {
	return master.Rows(ctx, r.db, scan, selectFlow+` WHERE NOT f.is_deleted`+groupBy+` ORDER BY f.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Flow, error) {
	if forUpdate {
		var id string
		if err := r.db.Q(ctx).QueryRow(ctx, `SELECT id FROM pm.custom_flow WHERE lower(code) = lower($1) AND NOT is_deleted FOR UPDATE`, code).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Flow{}, master.ErrNotFound
			}
			return Flow{}, err
		}
	}
	return master.One(ctx, r.db, scan, selectFlow+` WHERE lower(f.code) = lower($1) AND NOT f.is_deleted`+groupBy, code)
}

func (r repo) byID(ctx context.Context, id string) (Flow, error) {
	return master.One(ctx, r.db, scan, selectFlow+` WHERE f.id = $1`+groupBy, id)
}

func (r repo) saveSteps(ctx context.Context, id string, steps []string) error {
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM pm.custom_flow_step WHERE custom_flow_id = $1`, id); err != nil {
		return err
	}
	for i, pid := range steps {
		if _, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO pm.custom_flow_step (custom_flow_id, seq, position_id) VALUES ($1, $2, $3)`, id, i+1, pid); err != nil {
			return err
		}
	}
	return nil
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Flow, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.custom_flow (code, name, description, is_active, valid_from, valid_to, created_by,
		sop, approval_rule_id, scope_unit_id) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Sop, in.RuleID, in.ScopeUnitID).Scan(&id); err != nil {
		return Flow{}, err
	}
	if err := r.saveSteps(ctx, id, in.StepIDs); err != nil {
		return Flow{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Flow, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.custom_flow SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, sop = $7, approval_rule_id = $8, scope_unit_id = $9, updated_at = now(), updated_by = $10, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Sop, in.RuleID, in.ScopeUnitID, actor); err != nil {
		return Flow{}, err
	}
	if err := r.saveSteps(ctx, id, in.StepIDs); err != nil {
		return Flow{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "custom_flow", id, actor)
}
