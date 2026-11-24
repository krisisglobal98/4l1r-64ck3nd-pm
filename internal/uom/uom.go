// Package uom is the master "Satuan" (Logistik & Pengiriman, menu pm.uom): units of measure for cargo, stock and
// tariffs, with the UN/ECE Recommendation 20 code and the factor to the dimension's base unit (kg, m³, m, m², piece,
// hour), so 1 TNE = 1000 KGM. A unit without a fixed factor (a carton: pieces differ per item) leaves it empty. A new
// company starts with the common units.
package uom

import (
	"context"
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

var Dimensions = []string{"mass", "volume", "length", "area", "count", "time", "other"}

type Uom struct {
	record.Base
	UneceCode *string  `json:"uneceCode"`
	Dimension string   `json:"dimension"`
	Factor    *float64 `json:"factor"`
}

type Input struct {
	record.Input
	UneceCode *string
	Dimension string
	Factor    *float64
}

var unece = regexp.MustCompile(`^[0-9A-Z]{2,3}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if s := record.OptionalText(v, "uneceCode", "Kode UN/ECE", 3, &p); s != nil && !p.Has("uneceCode") {
		u := strings.ToUpper(*s)
		in.UneceCode = &u
		if !unece.MatchString(u) {
			p.Add("uneceCode", "Kode UN/ECE (Rec. 20) berisi 2–3 huruf/angka, mis. KGM untuk kilogram.")
		}
	}
	in.Dimension = record.Choice(v, "dimension", "Besaran", Dimensions, &p)
	in.Factor = record.Decimal(v, "factor", "Faktor ke satuan dasar", 0.0000000001, 99999999999999, 10, false, &p)
	if in.Dimension == "other" && in.Factor != nil {
		p.Add("factor", "Besaran lain tidak punya satuan dasar; kosongkan faktornya.")
	}
	return in, p
}

func values(x Uom) record.Values {
	v := record.ValuesOf(x.Base)
	v["uneceCode"], v["dimension"], v["factor"] = record.Opt(x.UneceCode), x.Dimension, record.Opt(x.Factor)
	return v
}

func New(db *database.DB) *master.Service[Uom, Input] {
	return &master.Service[Uom, Input]{
		Spec: master.Spec[Uom, Input]{
			Menu: "pm.uom", Table: "uom", Noun: "Satuan", Import: importSpec(db),
			Base: func(x *Uom) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Uom, p record.Problems) (record.Problems, error) {
				if in.UneceCode == nil || p.Has("uneceCode") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				other, err := refs.Duplicate(ctx, db, "uom", "unece_code", *in.UneceCode, self, false)
				if other != "" {
					p.Add("uneceCode", fmt.Sprintf("Kode UN/ECE %s sudah dipakai satuan %s.", *in.UneceCode, other))
				}
				return p, err
			},
		},
		Repo: master.TableRepo[Uom, Input]{DB: db, Table: "uom",
			Cols: []string{"unece_code", "dimension", "factor::float8"},
			Scan: func(row pgx.Row) (Uom, error) {
				var x Uom
				return x, x.Scan(row, &x.UneceCode, &x.Dimension, &x.Factor)
			},
			Input: func(in *Input) *record.Input { return &in.Input },
			Args:  func(in Input) []any { return []any{in.UneceCode, in.Dimension, in.Factor} },
			Order: "dimension, factor NULLS LAST, code"},
		DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}
