// Package orgunit is the Portal Master master "Unit Organisasi" (menu pm.orgUnit): the organization tree of
// group, region, wilayah, company and operational units. Its relations live in this service (levels, unit types,
// the parent unit), so they are checked here and stored as foreign keys; the API names them by code.
package orgunit

import (
	"alir.local/pm-service/internal/platform/record"
)

// Menu is the permission catalog menu of this master.
const Menu = "pm.orgUnit"

// Table is the master table, also its row in pm.code_prefix (the prefix of generated codes).
const Table = "org_unit"

// OperationalLevel is the level whose units have a unit type (Unit Operasional); built in, as in ALIR.
const OperationalLevel = "unit"

// Unit is the standard record followed by the fields of the Unit Organisasi page. Every relation is answered
// with its id and its code.
type Unit struct {
	record.Base
	LevelID      string  `json:"levelId"`
	LevelCode    string  `json:"levelCode"`
	ParentID     *string `json:"parentId"`
	ParentCode   *string `json:"parentCode"`
	UnitTypeID   *string `json:"unitTypeId"`
	UnitTypeCode *string `json:"unitTypeCode"`
	Location     *string `json:"location"`
}

// Input is what the form may set; relations by code.
type Input struct {
	record.Input
	LevelCode    string
	ParentCode   *string
	UnitTypeCode *string
	Location     *string
}

// Refs are the relations of an Input, resolved to ids.
type Refs struct {
	LevelID    string
	ParentID   *string
	UnitTypeID *string
}

const (
	maxRefCode  = record.MaxCode
	maxLocation = 80
)

func valuesOf(u Unit) record.Values {
	v := record.ValuesOf(u.Base)
	v["levelCode"] = u.LevelCode
	v["parentCode"], v["unitTypeCode"], v["location"] = ptr(u.ParentCode), ptr(u.UnitTypeCode), ptr(u.Location)
	return v
}

func ptr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// Parse checks the form values and returns what they set, or the problems per field. The code is not read; the
// relations are only read here, the service checks them against the tree.
func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.LevelCode = record.RequiredText(v, "levelCode", "Level", maxRefCode, &p)
	in.ParentCode = record.OptionalText(v, "parentCode", "Induk", maxRefCode, &p)
	in.UnitTypeCode = record.OptionalText(v, "unitTypeCode", "Jenis unit", maxRefCode, &p)
	in.Location = record.OptionalText(v, "location", "Lokasi", maxLocation, &p)
	return in, p
}
