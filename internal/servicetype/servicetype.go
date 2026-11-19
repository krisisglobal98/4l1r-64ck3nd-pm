// Package servicetype is the master "Moda & Layanan" (Logistik & Pengiriman, menu pm.serviceType): the services a
// forwarder sells (sea FCL/LCL, bulk, air freight, trucking FTL/LTL, customs clearance, warehousing, …) with their
// transport mode and load type. A job sheet, a quotation and a tariff name their service here. A new company starts
// with the common ones.
//
// Rules: container loads (FCL, LCL, bulk, breakbulk) go by sea or multimodal (FCL also by rail), truck loads (FTL,
// LTL) by land or multimodal; FCL needs containers.
package servicetype

import (
	"slices"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

var (
	Modes     = []string{"sea", "air", "land", "rail", "multimodal", "none"}
	LoadTypes = []string{"fcl", "lcl", "bulk", "breakbulk", "ftl", "ltl", "general", "none"}
)

type ServiceType struct {
	record.Base
	Mode            string `json:"mode"`
	LoadType        string `json:"loadType"`
	NeedsContainer  bool   `json:"needsContainer"`
	IncludesCustoms bool   `json:"includesCustoms"`
}

type Input struct {
	record.Input
	Mode, LoadType          string
	NeedsContainer, Customs bool
}

// Fits says whether a load type can go by a mode.
func Fits(mode, load string) bool {
	switch load {
	case "fcl":
		return slices.Contains([]string{"sea", "multimodal", "rail"}, mode)
	case "lcl", "bulk", "breakbulk":
		return mode == "sea" || mode == "multimodal"
	case "ftl", "ltl":
		return mode == "land" || mode == "multimodal"
	}
	return true
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Mode = record.Choice(v, "mode", "Moda", Modes, &p)
	in.LoadType = record.ChoiceOr(v, "loadType", "Jenis muatan", LoadTypes, "none", &p)
	if !p.Has("mode") && !p.Has("loadType") && !Fits(in.Mode, in.LoadType) {
		p.Add("loadType", "Jenis muatan ini tidak cocok dengan modanya (FCL/LCL/bulk lewat laut, FTL/LTL lewat darat).")
	}
	in.NeedsContainer = record.Bool(v, "needsContainer", "Pakai kontainer", in.LoadType == "fcl", &p)
	if in.LoadType == "fcl" {
		in.NeedsContainer = true
	}
	in.Customs = record.Bool(v, "includesCustoms", "Termasuk kepabeanan", false, &p)
	return in, p
}

func values(x ServiceType) record.Values {
	v := record.ValuesOf(x.Base)
	v["mode"], v["loadType"], v["needsContainer"], v["includesCustoms"] = x.Mode, x.LoadType, x.NeedsContainer, x.IncludesCustoms
	return v
}

func New(db *database.DB) *master.Service[ServiceType, Input] {
	return &master.Service[ServiceType, Input]{
		Spec: master.Spec[ServiceType, Input]{
			Menu: "pm.serviceType", Table: "service_type", Noun: "Layanan",
			Base: func(x *ServiceType) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
		},
		Repo: master.TableRepo[ServiceType, Input]{DB: db, Table: "service_type",
			Cols: []string{"mode", "load_type", "needs_container", "includes_customs"},
			Scan: func(row pgx.Row) (ServiceType, error) {
				var x ServiceType
				return x, x.Scan(row, &x.Mode, &x.LoadType, &x.NeedsContainer, &x.IncludesCustoms)
			},
			Input: func(in *Input) *record.Input { return &in.Input },
			Args:  func(in Input) []any { return []any{in.Mode, in.LoadType, in.NeedsContainer, in.Customs} }},
		DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}
