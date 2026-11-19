// Package packaging is the master "Kemasan" (Logistik & Pengiriman, menu pm.packaging): kinds of package (carton, bag,
// drum, pallet, …) with the UN/ECE Recommendation 21 code that customs documents and e-manifests use. A new company
// starts with the common kinds.
package packaging

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

type Packaging struct {
	record.Base
	UneceCode   *string  `json:"uneceCode"`
	MaxWeightKg *float64 `json:"maxWeightKg"`
}

type Input struct {
	record.Input
	UneceCode   *string
	MaxWeightKg *float64
}

var unece = regexp.MustCompile(`^[0-9A-Z]{2}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if s := record.OptionalText(v, "uneceCode", "Kode UN/ECE", 2, &p); s != nil && !p.Has("uneceCode") {
		u := strings.ToUpper(*s)
		in.UneceCode = &u
		if !unece.MatchString(u) {
			p.Add("uneceCode", "Kode UN/ECE (Rec. 21) berisi 2 huruf/angka, mis. CT untuk karton.")
		}
	}
	in.MaxWeightKg = record.Decimal(v, "maxWeightKg", "Berat isi maks. (kg)", 0.01, 99999999, 2, false, &p)
	return in, p
}

func values(x Packaging) record.Values {
	v := record.ValuesOf(x.Base)
	v["uneceCode"], v["maxWeightKg"] = record.Opt(x.UneceCode), record.Opt(x.MaxWeightKg)
	return v
}

func New(db *database.DB) *master.Service[Packaging, Input] {
	return &master.Service[Packaging, Input]{
		Spec: master.Spec[Packaging, Input]{
			Menu: "pm.packaging", Table: "packaging", Noun: "Kemasan",
			Base: func(x *Packaging) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Packaging, p record.Problems) (record.Problems, error) {
				if in.UneceCode == nil || p.Has("uneceCode") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				other, err := refs.Duplicate(ctx, db, "packaging", "unece_code", *in.UneceCode, self, false)
				if other != "" {
					p.Add("uneceCode", fmt.Sprintf("Kode UN/ECE %s sudah dipakai kemasan %s.", *in.UneceCode, other))
				}
				return p, err
			},
		},
		Repo: master.TableRepo[Packaging, Input]{DB: db, Table: "packaging",
			Cols: []string{"unece_code", "max_weight_kg::float8"},
			Scan: func(row pgx.Row) (Packaging, error) {
				var x Packaging
				return x, x.Scan(row, &x.UneceCode, &x.MaxWeightKg)
			},
			Input: func(in *Input) *record.Input { return &in.Input },
			Args:  func(in Input) []any { return []any{in.UneceCode, in.MaxWeightKg} }},
		DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}
