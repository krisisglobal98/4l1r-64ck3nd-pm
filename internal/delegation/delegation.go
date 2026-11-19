// Package delegation is the Portal Master master "Delegasi" (menu pm.delegation): while the holder of a position is
// away, a delegate signs in its place for a period, for every document type or only the listed ones. The name is
// the reason; valid_from/valid_to are the period (both required); the delegate is an employee (Karyawan), never
// the holder of the position itself.
package delegation

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

type Delegation struct {
	record.Base
	PositionID           string   `json:"positionId"`
	PositionCode         string   `json:"positionCode"`
	DelegateEmployeeID   *string  `json:"delegateEmployeeId"`
	DelegateEmployeeCode *string  `json:"delegateEmployeeCode"`
	RuleCodes            []string `json:"ruleCodes"` // empty = every document type
	ruleIDs              []string
}

type Input struct {
	record.Input
	PositionCode string
	DelegateCode string
	DelegateID   string
	RuleCodes    []string
	PositionID   string
	RuleIDs      []string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if in.ValidFrom == nil && !p.Has("validFrom") {
		p.Add("validFrom", "Berlaku mulai wajib diisi.")
	}
	if in.ValidTo == nil && !p.Has("validTo") {
		p.Add("validTo", "Berlaku sampai wajib diisi: delegasi tidak boleh tanpa batas waktu.")
	}
	in.PositionCode = record.RequiredText(v, "positionCode", "Posisi pemberi", record.MaxCode, &p)
	in.DelegateCode = record.RequiredText(v, "delegateEmployeeCode", "Penerima delegasi", record.MaxCode, &p)
	in.RuleCodes = record.Codes(v, "ruleCodes", "Jenis dokumen", &p)
	return in, p
}

func values(d Delegation) record.Values {
	v := record.ValuesOf(d.Base)
	v["positionCode"] = d.PositionCode
	v["delegateEmployeeCode"] = nil
	if d.DelegateEmployeeCode != nil {
		v["delegateEmployeeCode"] = *d.DelegateEmployeeCode
	}
	codes := []any{}
	for _, c := range d.RuleCodes {
		codes = append(codes, c)
	}
	v["ruleCodes"] = codes
	return v
}

func New(db *database.DB) *master.Service[Delegation, Input] {
	return &master.Service[Delegation, Input]{
		Spec: master.Spec[Delegation, Input]{
			Menu: "pm.delegation", Table: "delegation", Noun: "Delegasi",
			Base:    func(d *Delegation) *record.Base { return &d.Base },
			Values:  values,
			Parse:   Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Delegation, p record.Problems) (record.Problems, error) {
				var kept *string
				if stored != nil {
					kept = &stored.PositionID
				}
				if in.PositionCode != "" {
					pos, err := refs.Pick(ctx, db, "position", "positionCode", "Posisi", &in.PositionCode, kept, &p)
					if err != nil {
						return p, err
					}
					if pos != nil {
						in.PositionID = pos.ID
					}
				}
				if in.DelegateCode != "" {
					var keptDelegate *string
					if stored != nil {
						keptDelegate = stored.DelegateEmployeeID
					}
					emp, err := refs.Pick(ctx, db, "employee", "delegateEmployeeCode", "Karyawan", &in.DelegateCode, keptDelegate, &p)
					if err != nil {
						return p, err
					}
					if emp != nil {
						in.DelegateID = emp.ID
						var holder *string
						if in.PositionID != "" {
							if err := db.Q(ctx).QueryRow(ctx, `SELECT holder_employee_id FROM pm.position WHERE id = $1`, in.PositionID).Scan(&holder); err != nil {
								return p, err
							}
						}
						if holder != nil && *holder == emp.ID {
							p.Add("delegateEmployeeCode", fmt.Sprintf("%s adalah pemegang posisi itu sendiri; pilih karyawan lain.", emp.Name))
						}
					}
				}
				for _, c := range in.RuleCodes {
					var keptRule *string
					r, err := refs.Lookup(ctx, db, "approval_rule", c)
					if err == nil && stored != nil && slices.Contains(stored.ruleIDs, r.ID) {
						keptRule = &r.ID
					}
					rule, err := refs.Pick(ctx, db, "approval_rule", "ruleCodes", "Jenis dokumen", &c, keptRule, &p)
					if err != nil {
						return p, err
					}
					if rule != nil {
						in.RuleIDs = append(in.RuleIDs, rule.ID)
					}
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var selectDelegation = `SELECT ` + record.ColumnsAs("d") + `, d.position_id, x.code, d.delegate_employee_id, e.code,
	coalesce(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.id IS NOT NULL), '{}'),
	coalesce(array_agg(r.id::text ORDER BY r.code) FILTER (WHERE r.id IS NOT NULL), '{}')
	FROM pm.delegation d JOIN pm.position x ON x.id = d.position_id LEFT JOIN pm.employee e ON e.id = d.delegate_employee_id
	LEFT JOIN pm.delegation_rule dr ON dr.delegation_id = d.id LEFT JOIN pm.approval_rule r ON r.id = dr.approval_rule_id`

const groupBy = ` GROUP BY d.id, x.code, e.code`

func scan(row pgx.Row) (Delegation, error) {
	var d Delegation
	return d, d.Scan(row, &d.PositionID, &d.PositionCode, &d.DelegateEmployeeID, &d.DelegateEmployeeCode, &d.RuleCodes, &d.ruleIDs)
}

func (r repo) List(ctx context.Context) ([]Delegation, error) {
	return master.Rows(ctx, r.db, scan, selectDelegation+` WHERE NOT d.is_deleted`+groupBy+` ORDER BY d.valid_from DESC, d.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Delegation, error) {
	if forUpdate { // lock the row first: FOR UPDATE does not go with GROUP BY
		var id string
		if err := r.db.Q(ctx).QueryRow(ctx, `SELECT id FROM pm.delegation WHERE lower(code) = lower($1) AND NOT is_deleted FOR UPDATE`, code).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Delegation{}, master.ErrNotFound
			}
			return Delegation{}, err
		}
	}
	return master.One(ctx, r.db, scan, selectDelegation+` WHERE lower(d.code) = lower($1) AND NOT d.is_deleted`+groupBy, code)
}

func (r repo) byID(ctx context.Context, id string) (Delegation, error) {
	return master.One(ctx, r.db, scan, selectDelegation+` WHERE d.id = $1`+groupBy, id)
}

func (r repo) saveRules(ctx context.Context, id string, ruleIDs []string) error {
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM pm.delegation_rule WHERE delegation_id = $1`, id); err != nil {
		return err
	}
	for _, rid := range ruleIDs {
		if _, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO pm.delegation_rule (delegation_id, approval_rule_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, rid); err != nil {
			return err
		}
	}
	return nil
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Delegation, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.delegation (code, name, description, is_active, valid_from, valid_to, created_by, position_id, delegate_employee_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.PositionID, in.DelegateID).Scan(&id); err != nil {
		return Delegation{}, err
	}
	if err := r.saveRules(ctx, id, in.RuleIDs); err != nil {
		return Delegation{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Delegation, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.delegation SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, position_id = $7, delegate_employee_id = $9, updated_at = now(), updated_by = $8, version = version + 1
		WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.PositionID, actor, in.DelegateID); err != nil {
		return Delegation{}, err
	}
	if err := r.saveRules(ctx, id, in.RuleIDs); err != nil {
		return Delegation{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "delegation", id, actor)
}
