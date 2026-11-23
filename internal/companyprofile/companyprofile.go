// Package companyprofile is the master "Profil Perusahaan" (Organisasi, menu pm.orgUnit): the identity a company prints
// on its documents — legal and trade name, NPWP, address, contacts and logo — one per owning company unit (or one for
// the tenant). The profile marked "dipakai di dokumen" heads documents that name no company (Freight Forwarding
// invoices and job sheets read it through GET /pm/company-profile/print, open to every user of the tenant).
package companyprofile

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type Profile struct {
	record.Base
	TradeName    *string `json:"tradeName"`
	Npwp         *string `json:"npwp"`
	Address      *string `json:"address"`
	City         *string `json:"city"`
	CountryID    *string `json:"countryId"`
	CountryCode  *string `json:"countryCode"`
	CountryName  *string `json:"countryName"`
	Phone        *string `json:"phone"`
	Fax          *string `json:"fax"`
	Email        *string `json:"email"`
	Website      *string `json:"website"`
	Logo         *string `json:"logo"`
	UnitID       *string `json:"unitId"`
	UnitCode     *string `json:"unitCode"`
	PrintDefault bool    `json:"printDefault"`
}

type Input struct {
	record.Input
	TradeName, Npwp, Address, City, CountryCode, Phone, Fax, Email, Website, Logo, UnitCode *string
	CountryID, UnitID                                                                       *string
	PrintDefault                                                                            bool
}

// MaxLogo is the longest logo data URL kept (about 400 KB of image).
const MaxLogo = 560000

var logoURL = regexp.MustCompile(`^data:image/(png|jpeg|svg\+xml|webp);base64,[A-Za-z0-9+/=]+$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.TradeName = record.OptionalText(v, "tradeName", "Nama dagang", 150, &p)
	if in.Npwp = record.OptionalText(v, "npwp", "NPWP", 30, &p); in.Npwp != nil && !p.Has("npwp") {
		d := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, *in.Npwp)
		if len(d) == 15 {
			d = "0" + d
		}
		if len(d) != 16 {
			p.Add("npwp", "NPWP berisi 16 angka (NPWP 15 angka lama diterima dan diubah otomatis).")
		}
		in.Npwp = &d
	}
	in.Address = record.OptionalText(v, "address", "Alamat", 500, &p)
	in.City = record.OptionalText(v, "city", "Kota", 100, &p)
	in.CountryCode = record.OptionalText(v, "countryCode", "Negara", record.MaxCode, &p)
	in.Phone = record.OptionalPhone(v, "phone", "Telepon", &p)
	in.Fax = record.OptionalPhone(v, "fax", "Faks", &p)
	in.Email = record.OptionalEmail(v, "email", "Email", &p)
	in.Website = record.OptionalURL(v, "website", "Situs web", 200, &p)
	if s, _ := record.Text(v["logo"]); s != "" {
		switch {
		case len(s) > MaxLogo:
			p.Add("logo", "Logo paling besar sekitar 400 KB.")
		case !logoURL.MatchString(s):
			p.Add("logo", "Logo berupa gambar PNG, JPEG, SVG, atau WebP.")
		default:
			in.Logo = &s
		}
	}
	in.UnitCode = record.OptionalText(v, "unitCode", "Perusahaan", record.MaxCode, &p)
	in.PrintDefault = record.Bool(v, "printDefault", "Dipakai di dokumen", false, &p)
	return in, p
}

func values(x Profile) record.Values {
	v := record.ValuesOf(x.Base)
	for k, s := range map[string]*string{"tradeName": x.TradeName, "npwp": x.Npwp, "address": x.Address, "city": x.City, "countryCode": x.CountryCode,
		"phone": x.Phone, "fax": x.Fax, "email": x.Email, "website": x.Website, "logo": x.Logo, "unitCode": x.UnitCode} {
		v[k] = record.Opt(s)
	}
	v["printDefault"] = x.PrintDefault
	return v
}

func New(db *database.DB) *master.Service[Profile, Input] {
	return &master.Service[Profile, Input]{
		Spec: master.Spec[Profile, Input]{
			Menu: "pm.orgUnit", Table: "company_profile", Noun: "Profil perusahaan",
			Base: func(x *Profile) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Profile, p record.Problems) (record.Problems, error) {
				self := ""
				var keptCountry, keptUnit *string
				if stored != nil {
					self, keptCountry, keptUnit = stored.ID, stored.CountryID, stored.UnitID
				}
				c, err := refs.Pick(ctx, db, "country", "countryCode", "Negara", in.CountryCode, keptCountry, &p)
				if err != nil {
					return p, err
				}
				if c != nil {
					in.CountryID = &c.ID
				}
				u, err := refs.Pick(ctx, db, "org_unit", "unitCode", "Perusahaan", in.UnitCode, keptUnit, &p)
				if err != nil {
					return p, err
				}
				if u != nil {
					in.UnitID = &u.ID
				}
				if !p.Has("unitCode") {
					var other string
					switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.company_profile WHERE org_unit_id IS NOT DISTINCT FROM $1::uuid AND NOT is_deleted AND id::text <> $2`,
						in.UnitID, self).Scan(&other); {
					case err == nil:
						p.Add("unitCode", fmt.Sprintf("Perusahaan ini sudah punya profil %s.", other))
					case !errors.Is(err, pgx.ErrNoRows):
						return p, err
					}
				}
				if in.PrintDefault {
					var other string
					switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.company_profile WHERE print_default AND NOT is_deleted AND id::text <> $1`, self).Scan(&other); {
					case err == nil:
						p.Add("printDefault", fmt.Sprintf("Profil %s sudah dipakai di dokumen; matikan di sana lebih dulu.", other))
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

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.trade_name, x.npwp, x.address, x.city, x.country_id, c.code, c.name, x.phone, x.fax, x.email, x.website,
	x.logo, x.org_unit_id, u.code, x.print_default
	FROM pm.company_profile x LEFT JOIN pm.country c ON c.id = x.country_id LEFT JOIN pm.org_unit u ON u.id = x.org_unit_id`

func scan(row pgx.Row) (Profile, error) {
	var x Profile
	return x, x.Scan(row, &x.TradeName, &x.Npwp, &x.Address, &x.City, &x.CountryID, &x.CountryCode, &x.CountryName, &x.Phone, &x.Fax, &x.Email, &x.Website,
		&x.Logo, &x.UnitID, &x.UnitCode, &x.PrintDefault)
}

func (r repo) List(ctx context.Context) ([]Profile, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Profile, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Profile, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Profile, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.company_profile (code, name, description, is_active, valid_from, valid_to, created_by,
		trade_name, npwp, address, city, country_id, phone, fax, email, website, logo, org_unit_id, print_default)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor,
		in.TradeName, in.Npwp, in.Address, in.City, in.CountryID, in.Phone, in.Fax, in.Email, in.Website, in.Logo, in.UnitID, in.PrintDefault).Scan(&id); err != nil {
		return Profile{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Profile, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.company_profile SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		trade_name = $7, npwp = $8, address = $9, city = $10, country_id = $11, phone = $12, fax = $13, email = $14, website = $15, logo = $16,
		org_unit_id = $17, print_default = $18, updated_at = now(), updated_by = $19, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.TradeName, in.Npwp, in.Address, in.City, in.CountryID, in.Phone, in.Fax,
		in.Email, in.Website, in.Logo, in.UnitID, in.PrintDefault, actor); err != nil {
		return Profile{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "company_profile", id, actor)
}

// PrintRoutes registers GET /pm/company-profile/print?unit=: the profile a document prints — the company unit's own,
// else the one marked for documents, else the only active one; 404 when none. Any signed-in user of the tenant may read
// it (it is what their documents show), unlike the master itself.
func PrintRoutes(mux *http.ServeMux, db *database.DB) {
	mux.Handle("GET /pm/company-profile/print", httpx.Handler(func(w http.ResponseWriter, req *http.Request) error {
		if _, ok := access.UserFrom(req.Context()); !ok {
			return &apperr.Error{Status: http.StatusUnauthorized, Code: "SIGNED_OUT", Message: "Sesi tidak ditemukan. Silakan masuk kembali."}
		}
		cond := `x.is_active AND NOT x.is_deleted`
		args := []any{}
		order := `x.print_default DESC, x.code`
		if u := strings.TrimSpace(req.URL.Query().Get("unit")); u != "" {
			args = append(args, u)
			order = `(lower(u.code) = lower($1)) DESC, ` + order
		}
		rows, err := master.Rows(req.Context(), db, scan, sel+` WHERE `+cond+` ORDER BY `+order+` LIMIT 2`, args...)
		if err != nil {
			return err
		}
		switch {
		case len(rows) == 0:
			return apperr.NotFound()
		case len(rows) > 1 && !rows[0].PrintDefault && (len(args) == 0 || rows[0].UnitCode == nil || !strings.EqualFold(*rows[0].UnitCode, args[0].(string))):
			// several profiles and none chosen for documents: do not guess
			return apperr.NotFound()
		}
		return httpx.JSON(w, http.StatusOK, rows[0])
	}))
}
