// Package employee is the master "Karyawan" (Employee Management, menu pm.employee): the people who hold
// positions and receive delegations. The name is the full name; the NIK is the company's employee number (one
// live employee per NIK). Full personal data stays with Employee Management.
package employee

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type Employee struct {
	record.Base
	Nik                  string  `json:"nik"`
	Email                *string `json:"email"` // Authentication sends the person's mail here
	JoinDate             *string `json:"joinDate"`
	EmploymentStatusCode string  `json:"employmentStatusCode"`
	EducationLevelCode   *string `json:"educationLevelCode"`
	WorkShiftCode        *string `json:"workShiftCode"`
	BankCode             *string `json:"bankCode"`
	statusID             string
	educationID          *string
	shiftID              *string
	bankID               *string
}

type Input struct {
	record.Input
	Nik                                string
	Email                              *string
	JoinDate                           *string
	StatusCode                         string
	EducationCode, ShiftCode, BankCode *string
	StatusID                           string
	EducationID, ShiftID, BankID       *string
}

const maxNik = 20

var nikPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Nik = record.RequiredText(v, "nik", "NIK", maxNik, &p)
	if in.Nik != "" && !p.Has("nik") && !nikPattern.MatchString(in.Nik) {
		p.Add("nik", "NIK berisi huruf, angka, titik, garis miring, - atau _ tanpa spasi.")
	}
	in.Email = record.OptionalText(v, "email", "Email", 254, &p)
	if in.Email != nil && !p.Has("email") {
		e := strings.ToLower(strings.TrimSpace(*in.Email))
		if !emailPattern.MatchString(e) {
			p.Add("email", "Email bukan alamat yang sah, mis. nama@perusahaan.co.id.")
		}
		in.Email = &e
	}
	in.JoinDate = record.OptionalDate(v, "joinDate", "Tanggal masuk", &p)
	in.StatusCode = record.RequiredText(v, "employmentStatusCode", "Status kepegawaian", record.MaxCode, &p)
	in.EducationCode = record.OptionalText(v, "educationLevelCode", "Pendidikan terakhir", record.MaxCode, &p)
	in.ShiftCode = record.OptionalText(v, "workShiftCode", "Shift kerja", record.MaxCode, &p)
	in.BankCode = record.OptionalText(v, "bankCode", "Bank gaji", record.MaxCode, &p)
	return in, p
}

func opt(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func values(x Employee) record.Values {
	v := record.ValuesOf(x.Base)
	v["nik"], v["joinDate"], v["employmentStatusCode"], v["email"] = x.Nik, opt(x.JoinDate), x.EmploymentStatusCode, opt(x.Email)
	v["educationLevelCode"], v["workShiftCode"], v["bankCode"] = opt(x.EducationLevelCode), opt(x.WorkShiftCode), opt(x.BankCode)
	return v
}

func New(db *database.DB) *master.Service[Employee, Input] {
	return &master.Service[Employee, Input]{
		Spec: master.Spec[Employee, Input]{
			Menu: "pm.employee", Table: "employee", Noun: "Karyawan",
			Base: func(x *Employee) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Employee, p record.Problems) (record.Problems, error) {
				var kStatus, kEdu, kShift, kBank *string
				if stored != nil {
					kStatus, kEdu, kShift, kBank = &stored.statusID, stored.educationID, stored.shiftID, stored.bankID
				}
				pick := func(table, field, label string, code, kept *string, into **string) error {
					r, err := refs.Pick(ctx, db, table, field, label, code, kept, &p)
					if r != nil {
						*into = &r.ID
					}
					return err
				}
				var status *string
				if in.StatusCode != "" {
					if err := pick("employment_status", "employmentStatusCode", "Status kepegawaian", &in.StatusCode, kStatus, &status); err != nil {
						return p, err
					}
				}
				if status != nil {
					in.StatusID = *status
				}
				for _, x := range []struct {
					table, field, label string
					code, kept          *string
					into                **string
				}{
					{"education_level", "educationLevelCode", "Pendidikan", in.EducationCode, kEdu, &in.EducationID},
					{"work_shift", "workShiftCode", "Shift", in.ShiftCode, kShift, &in.ShiftID},
					{"bank", "bankCode", "Bank", in.BankCode, kBank, &in.BankID},
				} {
					if err := pick(x.table, x.field, x.label, x.code, x.kept, x.into); err != nil {
						return p, err
					}
				}
				if in.Email != nil && !p.Has("email") {
					self := ""
					if stored != nil {
						self = stored.ID
					}
					var other string
					err := db.Q(ctx).QueryRow(ctx, `SELECT name FROM pm.employee WHERE lower(email) = lower($1) AND NOT is_deleted AND id::text <> $2`, *in.Email, self).Scan(&other)
					switch {
					case err == nil:
						p.Add("email", fmt.Sprintf("Email %s sudah dipakai %s.", *in.Email, other))
					case !errors.Is(err, pgx.ErrNoRows):
						return p, err
					}
				}
				if in.Nik != "" && !p.Has("nik") {
					self := ""
					if stored != nil {
						self = stored.ID
					}
					var other string
					err := db.Q(ctx).QueryRow(ctx, `SELECT name FROM pm.employee WHERE lower(nik) = lower($1) AND NOT is_deleted AND id::text <> $2`, in.Nik, self).Scan(&other)
					switch {
					case err == nil:
						p.Add("nik", fmt.Sprintf("NIK %s sudah dipakai %s.", in.Nik, other))
					case !errors.Is(err, pgx.ErrNoRows):
						return p, err
					}
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var selectEmployee = `SELECT ` + record.ColumnsAs("e") + `, e.nik, e.email, to_char(e.join_date, 'YYYY-MM-DD'), s.code, d.code, w.code, b.code,
	e.employment_status_id, e.education_level_id, e.work_shift_id, e.bank_id
	FROM pm.employee e JOIN pm.employment_status s ON s.id = e.employment_status_id
	LEFT JOIN pm.education_level d ON d.id = e.education_level_id LEFT JOIN pm.work_shift w ON w.id = e.work_shift_id
	LEFT JOIN pm.bank b ON b.id = e.bank_id`

func scan(row pgx.Row) (Employee, error) {
	var x Employee
	return x, x.Scan(row, &x.Nik, &x.Email, &x.JoinDate, &x.EmploymentStatusCode, &x.EducationLevelCode, &x.WorkShiftCode, &x.BankCode,
		&x.statusID, &x.educationID, &x.shiftID, &x.bankID)
}

func (r repo) List(ctx context.Context) ([]Employee, error) {
	return master.Rows(ctx, r.db, scan, selectEmployee+` WHERE NOT e.is_deleted ORDER BY lower(e.name), e.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Employee, error) {
	q := selectEmployee + ` WHERE lower(e.code) = lower($1) AND NOT e.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF e`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Employee, error) {
	return master.One(ctx, r.db, scan, selectEmployee+` WHERE e.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Employee, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.employee (code, name, description, is_active, valid_from, valid_to, created_by,
		nik, join_date, employment_status_id, education_level_id, work_shift_id, bank_id, email)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9::date, $10, $11, $12, $13, $14) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Nik, in.JoinDate, in.StatusID, in.EducationID, in.ShiftID, in.BankID, in.Email).Scan(&id); err != nil {
		return Employee{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Employee, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.employee SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, nik = $7, join_date = $8::date, employment_status_id = $9, education_level_id = $10, work_shift_id = $11,
		bank_id = $12, email = $14, updated_at = now(), updated_by = $13, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Nik, in.JoinDate, in.StatusID, in.EducationID, in.ShiftID, in.BankID, actor, in.Email); err != nil {
		return Employee{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "employee", id, actor)
}
