// Package documenttype is the master "Jenis Dokumen" (Dokumen, menu pm.documentType): the documents a shipment, a
// customs declaration or a sale carries (B/L, AWB, DO, packing list, invoice, PIB/PEB, SKA, phytosanitary, …), with
// the group, the BC form code of a customs document, the issuer, whether it applies to export, import or domestic
// moves, and whether it needs a number, has an expiry or must be the original. A new company starts with the common
// ones. A BC code is used once.
package documenttype

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

var (
	Groups  = []string{"shipping", "customs", "commercial", "certificate", "insurance", "other"}
	Issuers = []string{"carrier", "forwarder", "customs", "shipper", "government", "company", "other"}
)

type DocumentType struct {
	record.Base
	Group            string  `json:"group"`
	CustomsCode      *string `json:"customsCode"`
	Issuer           string  `json:"issuer"`
	ForExport        bool    `json:"forExport"`
	ForImport        bool    `json:"forImport"`
	ForDomestic      bool    `json:"forDomestic"`
	NumberRequired   bool    `json:"numberRequired"`
	HasExpiry        bool    `json:"hasExpiry"`
	OriginalRequired bool    `json:"originalRequired"`
}

type Input struct {
	record.Input
	Group, Issuer                                               string
	CustomsCode                                                 *string
	ForExport, ForImport, ForDomestic, Number, Expiry, Original bool
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Group = record.Choice(v, "group", "Kelompok dokumen", Groups, &p)
	if s := record.OptionalText(v, "customsCode", "Kode BC", 10, &p); s != nil {
		u := strings.ToUpper(strings.Join(strings.Fields(*s), " "))
		in.CustomsCode = &u
	}
	in.Issuer = record.ChoiceOr(v, "issuer", "Penerbit", Issuers, "other", &p)
	in.ForExport = record.Bool(v, "forExport", "Ekspor", true, &p)
	in.ForImport = record.Bool(v, "forImport", "Impor", true, &p)
	in.ForDomestic = record.Bool(v, "forDomestic", "Domestik", false, &p)
	if !in.ForExport && !in.ForImport && !in.ForDomestic {
		p.Add("forExport", "Pilih minimal satu: ekspor, impor, atau domestik.")
	}
	in.Number = record.Bool(v, "numberRequired", "Wajib nomor", true, &p)
	in.Expiry = record.Bool(v, "hasExpiry", "Ada masa berlaku", false, &p)
	in.Original = record.Bool(v, "originalRequired", "Wajib asli", false, &p)
	return in, p
}

func values(x DocumentType) record.Values {
	v := record.ValuesOf(x.Base)
	v["group"], v["customsCode"], v["issuer"], v["forExport"], v["forImport"], v["forDomestic"] = x.Group, record.Opt(x.CustomsCode), x.Issuer, x.ForExport, x.ForImport, x.ForDomestic
	v["numberRequired"], v["hasExpiry"], v["originalRequired"] = x.NumberRequired, x.HasExpiry, x.OriginalRequired
	return v
}

func New(db *database.DB) *master.Service[DocumentType, Input] {
	return &master.Service[DocumentType, Input]{
		Spec: master.Spec[DocumentType, Input]{
			Menu: "pm.documentType", Table: "document_type", Noun: "Jenis dokumen",
			Base: func(x *DocumentType) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *DocumentType, p record.Problems) (record.Problems, error) {
				if in.CustomsCode == nil || p.Has("customsCode") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				other, err := refs.Duplicate(ctx, db, "document_type", "customs_code", *in.CustomsCode, self, false)
				if other != "" {
					p.Add("customsCode", fmt.Sprintf("Kode %s sudah dipakai %s.", *in.CustomsCode, other))
				}
				return p, err
			},
		},
		Repo: master.TableRepo[DocumentType, Input]{DB: db, Table: "document_type",
			Cols: []string{"doc_group", "customs_code", "issuer", "for_export", "for_import", "for_domestic", "number_required", "has_expiry", "original_required"},
			Scan: func(row pgx.Row) (DocumentType, error) {
				var x DocumentType
				return x, x.Scan(row, &x.Group, &x.CustomsCode, &x.Issuer, &x.ForExport, &x.ForImport, &x.ForDomestic, &x.NumberRequired, &x.HasExpiry, &x.OriginalRequired)
			},
			Input: func(in *Input) *record.Input { return &in.Input },
			Args: func(in Input) []any {
				return []any{in.Group, in.CustomsCode, in.Issuer, in.ForExport, in.ForImport, in.ForDomestic, in.Number, in.Expiry, in.Original}
			},
			Order: "doc_group, code"},
		DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}
