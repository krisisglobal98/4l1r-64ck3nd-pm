// Package incoterm is the master "Incoterms" (Logistik & Pengiriman, menu pm.incoterm): the ICC delivery terms a sales
// or purchase contract and a shipment name: who pays the main carriage and the insurance, who clears import, and where
// the risk passes to the buyer. A new company starts with the eleven terms of Incoterms 2020; one live record per term
// and edition.
package incoterm

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
)

type Incoterm struct {
	record.Base
	Term               string  `json:"term"`
	Edition            int     `json:"edition"`
	AnyMode            bool    `json:"anyMode"` // false: sea and inland waterway only
	SellerPaysCarriage bool    `json:"sellerPaysCarriage"`
	SellerInsures      bool    `json:"sellerInsures"`
	SellerClearsImport bool    `json:"sellerClearsImport"`
	RiskPoint          *string `json:"riskPoint"`
}

type Input struct {
	record.Input
	Term                                                     string
	Edition                                                  int
	AnyMode, SellerPaysCarriage, SellerInsures, SellerClears bool
	RiskPoint                                                *string
}

var term = regexp.MustCompile(`^[A-Z]{3}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if in.Term = strings.ToUpper(record.RequiredText(v, "term", "Kode Incoterm", 3, &p)); in.Term != "" && !p.Has("term") && !term.MatchString(in.Term) {
		p.Add("term", "Kode Incoterm berisi 3 huruf, mis. FOB.")
	}
	in.Edition = 2020
	if e := record.OptionalInt(v, "edition", "Edisi", 2010, &p); e != nil {
		if *e != 2010 && *e != 2020 {
			p.Add("edition", "Edisi Incoterms 2010 atau 2020.")
		}
		in.Edition = *e
	}
	in.AnyMode = record.Bool(v, "anyMode", "Semua moda", true, &p)
	in.SellerPaysCarriage = record.Bool(v, "sellerPaysCarriage", "Penjual bayar angkutan utama", false, &p)
	in.SellerInsures = record.Bool(v, "sellerInsures", "Penjual asuransikan", false, &p)
	in.SellerClears = record.Bool(v, "sellerClearsImport", "Penjual urus impor", false, &p)
	in.RiskPoint = record.OptionalText(v, "riskPoint", "Titik peralihan risiko", 200, &p)
	if in.SellerInsures && !in.SellerPaysCarriage && !p.Has("sellerInsures") {
		p.Add("sellerInsures", "Penjual yang mengasuransikan juga membayar angkutan utama (CIF, CIP).")
	}
	return in, p
}

func values(x Incoterm) record.Values {
	v := record.ValuesOf(x.Base)
	v["term"], v["edition"], v["anyMode"], v["sellerPaysCarriage"], v["sellerInsures"], v["sellerClearsImport"], v["riskPoint"] =
		x.Term, x.Edition, x.AnyMode, x.SellerPaysCarriage, x.SellerInsures, x.SellerClearsImport, record.Opt(x.RiskPoint)
	return v
}

func New(db *database.DB) *master.Service[Incoterm, Input] {
	return &master.Service[Incoterm, Input]{
		Spec: master.Spec[Incoterm, Input]{
			Menu: "pm.incoterm", Table: "incoterm", Noun: "Incoterm", Global: true,
			Base: func(x *Incoterm) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Incoterm, p record.Problems) (record.Problems, error) {
				if in.Term == "" || p.Has("term") || p.Has("edition") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				var other string
				err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.incoterm WHERE term = $1 AND edition = $2 AND NOT is_deleted AND id::text <> $3`, in.Term, in.Edition, self).Scan(&other)
				switch {
				case err == nil:
					p.Add("term", fmt.Sprintf("%s edisi %d sudah ada (%s).", in.Term, in.Edition, other))
				case !errors.Is(err, pgx.ErrNoRows):
					return p, err
				}
				return p, nil
			},
		},
		Repo: master.TableRepo[Incoterm, Input]{DB: db, Table: "incoterm",
			Cols: []string{"term", "edition", "any_mode", "seller_pays_carriage", "seller_insures", "seller_clears_import", "risk_point"},
			Scan: func(row pgx.Row) (Incoterm, error) {
				var x Incoterm
				return x, x.Scan(row, &x.Term, &x.Edition, &x.AnyMode, &x.SellerPaysCarriage, &x.SellerInsures, &x.SellerClearsImport, &x.RiskPoint)
			},
			Input: func(in *Input) *record.Input { return &in.Input },
			Args: func(in Input) []any {
				return []any{in.Term, in.Edition, in.AnyMode, in.SellerPaysCarriage, in.SellerInsures, in.SellerClears, in.RiskPoint}
			},
			Order: "edition DESC, code"},
		DB: db, Codes: codeprefix.Global{DB: db, Prefix: "MIC", Digits: 3}, Events: outbox.Writer{DB: db},
	}
}
