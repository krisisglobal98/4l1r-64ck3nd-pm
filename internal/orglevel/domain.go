// Package orglevel is the Portal Master master "Level Organisasi" (menu pm.orgLevel): the levels of the
// organization tree, top to bottom. The rules match the ALIR form (masterSchema.ts `levels`).
package orglevel

import (
	"strings"

	"alir.local/pm-service/internal/platform/record"
)

// Menu is the permission catalog menu of this master.
const Menu = "pm.orgLevel"

// Table is the master table, also its row in pm.code_prefix (the prefix of generated codes).
const Table = "org_level"

// Level is the standard record followed by the fields of the Level Organisasi page.
type Level struct {
	record.Base
	Rank int `json:"rank"`
}

// Input is what the form may set.
type Input struct {
	record.Input
	Rank int
}

// builtIn are the levels the organization tree and the scope of user accounts are built on; units and job titles
// point to them, so they are not deleted (their names, ranks and status may still change).
var builtIn = map[string]bool{"group": true, "region": true, "wilayah": true, "company": true, "unit": true, "org1": true, "org2": true, "org3": true}

func IsBuiltIn(code string) bool { return builtIn[strings.ToLower(code)] }

func valuesOf(l Level) record.Values {
	v := record.ValuesOf(l.Base)
	v["rank"] = l.Rank
	return v
}

// Parse checks the form values and returns what they set, or the problems per field. The code is not read.
func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Rank = record.RequiredInt(v, "rank", "Urutan", 1, &p)
	return in, p
}
