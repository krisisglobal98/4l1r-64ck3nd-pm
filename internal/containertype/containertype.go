// Package containertype is the master "Jenis Kontainer" (Logistik & Pengiriman, menu pm.container): container sizes and
// types with their ISO 6346 size-type code, length, height, group, TEU and weights. Bookings, job sheets and tariffs
// count containers by these. A new company starts with the common types (20'/40' general, high cube, reefer, open top,
// flat rack, tank). The payload is the maximum gross weight minus the tare.
package containertype

import (
	"context"
	"fmt"
	"regexp"
	"slices"
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
	Lengths = []int{10, 20, 40, 45, 48, 53}
	Groups  = []string{"general", "reefer", "open_top", "flat_rack", "tank", "platform", "ventilated", "bulk", "other"}
	// size (length: 2 = 20', 4 = 40', L = 45', M = 48', P = 53'…; height), type, detail: 22G1, 45G1, L5G1
	isoCode = regexp.MustCompile(`^[0-9A-Z][0-9A-Z][A-Z][0-9A-Z]$`)
)

type ContainerType struct {
	record.Base
	IsoCode        *string  `json:"isoCode"`
	LengthFt       int      `json:"lengthFt"`
	Height         string   `json:"height"` // standard | high_cube
	Group          string   `json:"group"`
	Teu            float64  `json:"teu"`
	MaxGrossKg     *int     `json:"maxGrossKg"`
	TareKg         *int     `json:"tareKg"`
	PayloadKg      *int     `json:"payloadKg"` // max gross − tare, when both are known
	InnerVolumeCbm *float64 `json:"innerVolumeCbm"`
}

type Input struct {
	record.Input
	IsoCode        *string
	LengthFt       int
	Height, Group  string
	Teu            float64
	MaxGrossKg     *int
	TareKg         *int
	InnerVolumeCbm *float64
}

// DefaultTeu is the TEU of a length when the form leaves it empty.
func DefaultTeu(ft int) float64 {
	switch ft {
	case 10:
		return 0.5
	case 20:
		return 1
	case 45:
		return 2.25
	default:
		return 2
	}
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if s := record.OptionalText(v, "isoCode", "Kode ISO 6346", 4, &p); s != nil && !p.Has("isoCode") {
		u := strings.ToUpper(*s)
		in.IsoCode = &u
		if !isoCode.MatchString(u) {
			p.Add("isoCode", "Kode ISO 6346 berisi 4 karakter ukuran-tipe, mis. 22G1, 45G1 atau L5G1.")
		}
	}
	in.LengthFt = record.RequiredInt(v, "lengthFt", "Panjang (ft)", 1, &p)
	if !p.Has("lengthFt") && !slices.Contains(Lengths, in.LengthFt) {
		p.Add("lengthFt", "Panjang kontainer 10, 20, 40, 45, 48 atau 53 kaki.")
	}
	in.Height = record.ChoiceOr(v, "height", "Tinggi", []string{"standard", "high_cube"}, "standard", &p)
	in.Group = record.Choice(v, "group", "Kelompok", Groups, &p)
	in.Teu = DefaultTeu(in.LengthFt)
	if t := record.Decimal(v, "teu", "TEU", 0.01, 10, 2, false, &p); t != nil {
		in.Teu = *t
	}
	in.MaxGrossKg = record.OptionalInt(v, "maxGrossKg", "Berat kotor maks. (kg)", 1, &p)
	in.TareKg = record.OptionalInt(v, "tareKg", "Berat kosong (kg)", 0, &p)
	if in.MaxGrossKg != nil && in.TareKg != nil && *in.TareKg >= *in.MaxGrossKg {
		p.Add("tareKg", "Berat kosong harus kurang dari berat kotor maksimum.")
	}
	in.InnerVolumeCbm = record.Decimal(v, "innerVolumeCbm", "Volume dalam (m³)", 0.01, 9999, 2, false, &p)
	return in, p
}

func values(x ContainerType) record.Values {
	v := record.ValuesOf(x.Base)
	v["isoCode"], v["lengthFt"], v["height"], v["group"], v["teu"] = record.Opt(x.IsoCode), x.LengthFt, x.Height, x.Group, x.Teu
	v["maxGrossKg"], v["tareKg"], v["innerVolumeCbm"] = record.Opt(x.MaxGrossKg), record.Opt(x.TareKg), record.Opt(x.InnerVolumeCbm)
	return v
}

func scan(row pgx.Row) (ContainerType, error) {
	var x ContainerType
	if err := x.Scan(row, &x.IsoCode, &x.LengthFt, &x.Height, &x.Group, &x.Teu, &x.MaxGrossKg, &x.TareKg, &x.InnerVolumeCbm); err != nil {
		return x, err
	}
	if x.MaxGrossKg != nil && x.TareKg != nil {
		n := *x.MaxGrossKg - *x.TareKg
		x.PayloadKg = &n
	}
	return x, nil
}

func New(db *database.DB) *master.Service[ContainerType, Input] {
	return &master.Service[ContainerType, Input]{
		Spec: master.Spec[ContainerType, Input]{
			Menu: "pm.container", Table: "container_type", Noun: "Jenis kontainer", Import: importSpec(db),
			Base: func(x *ContainerType) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *ContainerType, p record.Problems) (record.Problems, error) {
				if in.IsoCode == nil || p.Has("isoCode") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				other, err := refs.Duplicate(ctx, db, "container_type", "iso_code", *in.IsoCode, self, false)
				if other != "" {
					p.Add("isoCode", fmt.Sprintf("Kode ISO %s sudah dipakai %s.", *in.IsoCode, other))
				}
				return p, err
			},
		},
		Repo: master.TableRepo[ContainerType, Input]{DB: db, Table: "container_type",
			Cols:  []string{"iso_code", "length_ft", "height", "type_group", "teu::float8", "max_gross_kg", "tare_kg", "inner_volume_cbm::float8"},
			Scan:  scan,
			Input: func(in *Input) *record.Input { return &in.Input },
			Args: func(in Input) []any {
				return []any{in.IsoCode, in.LengthFt, in.Height, in.Group, in.Teu, in.MaxGrossKg, in.TareKg, in.InnerVolumeCbm}
			},
			Order: "length_ft, code"},
		DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}
