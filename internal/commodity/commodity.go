// Package commodity is the master "Komoditas" (Logistik & Pengiriman, menu pm.commodity): kinds of cargo with their HS
// code (BTKI, digits only), whether they are dangerous goods (then the UN number and the IMDG/IATA class are required),
// whether they need temperature control, and the default unit and packaging a shipment line starts with.
package commodity

import (
	"context"
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

type Commodity struct {
	record.Base
	HsCode                *string `json:"hsCode"`
	Dangerous             bool    `json:"dangerous"`
	UnNumber              *string `json:"unNumber"`
	DgClass               *string `json:"dgClass"`
	TemperatureControlled bool    `json:"temperatureControlled"`
	UomID                 *string `json:"uomId"`
	UomCode               *string `json:"uomCode"`
	PackagingID           *string `json:"packagingId"`
	PackagingCode         *string `json:"packagingCode"`
}

type Input struct {
	record.Input
	HsCode, UnNumber, DgClass *string
	Dangerous, TempControlled bool
	UomCode, PackagingCode    *string
	UomID, PackagingID        *string
}

var (
	hs      = regexp.MustCompile(`^([0-9]{4}|[0-9]{6}|[0-9]{8}|[0-9]{10})$`)
	unNo    = regexp.MustCompile(`^[0-9]{4}$`)
	dgClass = regexp.MustCompile(`^[1-9](\.[1-6])?$`)
	hsSep   = strings.NewReplacer(".", "", " ", "", "-", "")
)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if s := record.OptionalText(v, "hsCode", "Kode HS", 14, &p); s != nil && !p.Has("hsCode") {
		d := hsSep.Replace(*s)
		in.HsCode = &d
		if !hs.MatchString(d) {
			p.Add("hsCode", "Kode HS berisi 4, 6, 8 atau 10 angka (titik boleh), mis. 1511.10.00.")
		}
	}
	in.Dangerous = record.Bool(v, "dangerous", "Barang berbahaya", false, &p)
	in.TempControlled = record.Bool(v, "temperatureControlled", "Butuh suhu terkendali", false, &p)
	if in.Dangerous {
		if s := record.OptionalText(v, "unNumber", "Nomor UN", 10, &p); s != nil && !p.Has("unNumber") {
			d := strings.TrimPrefix(strings.ToUpper(strings.ReplaceAll(*s, " ", "")), "UN")
			in.UnNumber = &d
			if !unNo.MatchString(d) {
				p.Add("unNumber", "Nomor UN berisi 4 angka, mis. 1203.")
			}
		} else if !p.Has("unNumber") {
			p.Add("unNumber", "Barang berbahaya wajib punya nomor UN.")
		}
		if in.DgClass = record.OptionalText(v, "dgClass", "Kelas bahaya", 3, &p); in.DgClass == nil && !p.Has("dgClass") {
			p.Add("dgClass", "Barang berbahaya wajib punya kelas bahaya (IMDG/IATA), mis. 3.")
		} else if in.DgClass != nil && !p.Has("dgClass") && !dgClass.MatchString(*in.DgClass) {
			p.Add("dgClass", "Kelas bahaya 1–9, boleh dengan divisi, mis. 2.1.")
		}
	}
	// A cargo that is not dangerous has no UN number or class, whatever the form sent.
	in.UomCode = record.OptionalText(v, "uomCode", "Satuan", record.MaxCode, &p)
	in.PackagingCode = record.OptionalText(v, "packagingCode", "Kemasan", record.MaxCode, &p)
	return in, p
}

func values(x Commodity) record.Values {
	v := record.ValuesOf(x.Base)
	v["hsCode"], v["dangerous"], v["unNumber"], v["dgClass"], v["temperatureControlled"] = record.Opt(x.HsCode), x.Dangerous, record.Opt(x.UnNumber), record.Opt(x.DgClass), x.TemperatureControlled
	v["uomCode"], v["packagingCode"] = record.Opt(x.UomCode), record.Opt(x.PackagingCode)
	return v
}

func New(db *database.DB) *master.Service[Commodity, Input] {
	return &master.Service[Commodity, Input]{
		Spec: master.Spec[Commodity, Input]{
			Menu: "pm.commodity", Table: "commodity", Noun: "Komoditas",
			Base: func(x *Commodity) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Commodity, p record.Problems) (record.Problems, error) {
				var keptUom, keptPack *string
				if stored != nil {
					keptUom, keptPack = stored.UomID, stored.PackagingID
				}
				u, err := refs.Pick(ctx, db, "uom", "uomCode", "Satuan", in.UomCode, keptUom, &p)
				if err != nil {
					return p, err
				}
				k, err := refs.Pick(ctx, db, "packaging", "packagingCode", "Kemasan", in.PackagingCode, keptPack, &p)
				if u != nil {
					in.UomID = &u.ID
				}
				if k != nil {
					in.PackagingID = &k.ID
				}
				return p, err
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.hs_code, x.is_dangerous, x.un_number, x.dg_class, x.temperature_controlled,
	x.uom_id, u.code, x.packaging_id, k.code FROM pm.commodity x LEFT JOIN pm.uom u ON u.id = x.uom_id LEFT JOIN pm.packaging k ON k.id = x.packaging_id`

func scan(row pgx.Row) (Commodity, error) {
	var x Commodity
	return x, x.Scan(row, &x.HsCode, &x.Dangerous, &x.UnNumber, &x.DgClass, &x.TemperatureControlled, &x.UomID, &x.UomCode, &x.PackagingID, &x.PackagingCode)
}

func (r repo) List(ctx context.Context) ([]Commodity, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Commodity, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Commodity, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Commodity, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.commodity (code, name, description, is_active, valid_from, valid_to, created_by,
		hs_code, is_dangerous, un_number, dg_class, temperature_controlled, uom_id, packaging_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor,
		in.HsCode, in.Dangerous, in.UnNumber, in.DgClass, in.TempControlled, in.UomID, in.PackagingID).Scan(&id); err != nil {
		return Commodity{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Commodity, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.commodity SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		hs_code = $7, is_dangerous = $8, un_number = $9, dg_class = $10, temperature_controlled = $11, uom_id = $12, packaging_id = $13,
		updated_at = now(), updated_by = $14, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo,
		in.HsCode, in.Dangerous, in.UnNumber, in.DgClass, in.TempControlled, in.UomID, in.PackagingID, actor); err != nil {
		return Commodity{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "commodity", id, actor)
}
