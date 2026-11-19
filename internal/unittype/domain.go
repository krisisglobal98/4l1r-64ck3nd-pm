// Package unittype is the Portal Master master "Jenis Unit Operasional" (menu pm.unitType): the kinds of units at
// the Unit Operasional level (estate, mill, plasma, …). The rules match the ALIR form (masterSchema.ts `unit-types`).
package unittype

import (
	"alir.local/pm-service/internal/platform/record"
)

// Menu is the permission catalog menu of this master.
const Menu = "pm.unitType"

// Table is the master table, also its row in pm.code_prefix (the prefix of generated codes).
const Table = "unit_type"

// UnitType is the standard record followed by the fields of the Jenis Unit page.
type UnitType struct {
	record.Base
	// HeadJabatanID/Code is the head job title of this kind of unit (Jabatan master), e.g. EM = Estate Manager.
	HeadJabatanID   *string `json:"headJabatanId"`
	HeadJabatanCode *string `json:"headJabatanCode"`
}

// Input is what the form may set; the head job title by code.
type Input struct {
	record.Input
	HeadJabatanCode *string
	HeadJabatanID   *string // resolved by the service
}

func valuesOf(u UnitType) record.Values {
	v := record.ValuesOf(u.Base)
	v["headJabatanCode"] = nil
	if u.HeadJabatanCode != nil {
		v["headJabatanCode"] = *u.HeadJabatanCode
	}
	return v
}

// Parse checks the form values and returns what they set, or the problems per field. The code is not read.
func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.HeadJabatanCode = record.OptionalText(v, "headJabatanCode", "Jabatan kepala", record.MaxCode, &p)
	return in, p
}
