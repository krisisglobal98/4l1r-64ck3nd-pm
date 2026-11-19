//go:build e2e

// Black-box tests of the Portal Master API: positive, negative, security and stress scenarios per master over HTTP
// against a running service, with the database read directly to check what was stored. `make test-e2e` runs them.
//
//	E2E_BASE_URL=http://localhost:8082 TEST_DATABASE_URL=… go test -tags e2e -count=1 -v ./test/e2e/
//
// Run the service against the TEST database only: the suite commits and cleans up after itself.
// One file per master (levels_test.go is the pilot); this file holds what they share.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	userID   = "00000000-0000-7000-8000-000000000001"
	systemID = "00000000-0000-7000-8000-000000000000"
)

var (
	baseURL string
	db      *pgxpool.Pool // the application role in tenant A: the suite sees what the service sees
	owner   *pgxpool.Pool // the table owner: reset() clears every tenant
	client  = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConns: 300, MaxIdleConnsPerHost: 300}}
	uuidV7  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	genCode = regexp.MustCompile(`^ML\d{3,}$`)
	utcTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)
)

func TestMain(m *testing.M) {
	baseURL = strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	dsn, appDSN := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_APP_DATABASE_URL")
	if baseURL == "" || dsn == "" || appDSN == "" {
		fmt.Println("E2E_BASE_URL, TEST_DATABASE_URL and TEST_APP_DATABASE_URL are required")
		os.Exit(2)
	}
	if !strings.Contains(dsn, "_test") || !strings.Contains(appDSN, "_test") {
		fmt.Println("refusing to run: TEST_DATABASE_URL and TEST_APP_DATABASE_URL must point to a *_test database")
		os.Exit(2)
	}
	var err error
	if owner, err = pgxpool.New(context.Background(), dsn); err != nil {
		panic(err)
	}
	cfg, err := pgxpool.ParseConfig(appDSN)
	if err != nil {
		panic(err)
	}
	// Every direct read and insert of the suite is in the migrated tenant, under Row-Level Security.
	cfg.ConnConfig.RuntimeParams["app.tenant_id"] = tenantA
	if db, err = pgxpool.NewWithConfig(context.Background(), cfg); err != nil {
		panic(err)
	}
	reset()
	code := m.Run()
	reset()
	db.Close()
	owner.Close()
	os.Exit(code)
}

// tenantA is the tenant migrated from before tenants existed (the default user's tenant).
const tenantA = "00000000-0000-7000-8000-0000000000a1"

// reset puts the test database back to the migration seed.
func reset() {
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM pm.outbox`,
		`DELETE FROM pm.custom_flow_step`, `DELETE FROM pm.custom_flow`,
		`DELETE FROM pm.delegation_rule`, `DELETE FROM pm.delegation`,
		`DELETE FROM pm.approval_rule_band`, `DELETE FROM pm.approval_rule`,
		`DELETE FROM pm.position`,
		`UPDATE pm.unit_type SET head_jabatan_id = NULL`,
		`DELETE FROM pm.jabatan`, `DELETE FROM pm.grade`,
		`DELETE FROM pm.employee`, `DELETE FROM pm.employment_status`, `DELETE FROM pm.education_level`, `DELETE FROM pm.work_shift`, `DELETE FROM pm.bank`,
		// Reference masters: the standard rows of the migrated tenant stay (as seeded), everything else goes.
		`DELETE FROM pm.city`, `DELETE FROM pm.payment_term`, `DELETE FROM pm.tax_type`,
		`DELETE FROM pm.country WHERE tenant_id <> '` + tenantA + `' OR created_by <> '` + systemID + `'`,
		`DELETE FROM pm.currency WHERE tenant_id <> '` + tenantA + `' OR created_by <> '` + systemID + `'`,
		`UPDATE pm.country SET is_active = true, is_deleted = false, deleted_at = NULL, deleted_by = NULL, updated_at = NULL, updated_by = NULL, version = 1`,
		`UPDATE pm.currency SET is_active = true, is_deleted = false, deleted_at = NULL, deleted_by = NULL, updated_at = NULL, updated_by = NULL, version = 1`,
		`DELETE FROM pm.org_unit`,
		`DELETE FROM pm.org_level WHERE code NOT IN ('group','region','wilayah','company','unit','org1','org2','org3')`,
		`UPDATE pm.org_level SET name = CASE code WHEN 'group' THEN 'Group' WHEN 'region' THEN 'Region' WHEN 'wilayah' THEN 'Wilayah'
		   WHEN 'company' THEN 'Perusahaan' WHEN 'unit' THEN 'Unit Operasional' WHEN 'org1' THEN 'Organisasi Level 1'
		   WHEN 'org2' THEN 'Organisasi Level 2' ELSE 'Organisasi Level 3' END,
		   rank = CASE code WHEN 'group' THEN 1 WHEN 'region' THEN 2 WHEN 'wilayah' THEN 3 WHEN 'company' THEN 4 WHEN 'unit' THEN 5
		   WHEN 'org1' THEN 6 WHEN 'org2' THEN 7 ELSE 8 END,
		   description = NULL, is_active = true, valid_from = NULL, valid_to = NULL, updated_at = NULL, updated_by = NULL, version = 1`,
		`DELETE FROM pm.unit_type WHERE code NOT IN ('EST','PKS','PLS','BLK','KTR')`,
		`UPDATE pm.unit_type SET name = CASE code WHEN 'EST' THEN 'Kebun Inti' WHEN 'PKS' THEN 'Pabrik Kelapa Sawit' WHEN 'PLS' THEN 'Kebun Plasma'
		   WHEN 'BLK' THEN 'Gudang Bulking' ELSE 'Kantor Perwakilan' END,
		   description = NULL, is_active = true, valid_from = NULL, valid_to = NULL, updated_at = NULL, updated_by = NULL, version = 1,
		   is_deleted = false, deleted_at = NULL, deleted_by = NULL`,
		// Tenants other than the migrated one (the isolation suite) leave nothing behind.
		`DELETE FROM pm.org_level WHERE tenant_id <> '` + tenantA + `'`, `DELETE FROM pm.unit_type WHERE tenant_id <> '` + tenantA + `'`,
		`DELETE FROM pm.code_prefix WHERE tenant_id <> '` + tenantA + `'`,
		`UPDATE pm.code_prefix SET last_number = 0, is_active = true WHERE code IN ('org_level', 'unit_type', 'org_unit', 'grade', 'jabatan', 'position', 'approval_rule', 'delegation', 'custom_flow', 'employment_status', 'education_level', 'work_shift', 'bank', 'employee', 'city', 'payment_term', 'tax_type')`,
		// The seeded currencies and countries keep their numbers: the counter continues after them.
		`UPDATE pm.code_prefix p SET last_number = (SELECT count(*) FROM pm.currency c WHERE c.tenant_id = p.tenant_id), is_active = true WHERE code = 'currency'`,
		`UPDATE pm.code_prefix p SET last_number = (SELECT count(*) FROM pm.country c WHERE c.tenant_id = p.tenant_id), is_active = true WHERE code = 'country'`,
	} {
		if _, err := owner.Exec(ctx, q); err != nil {
			panic(err)
		}
	}
}

/* ---------------- HTTP helpers ---------------- */

type resp struct {
	Status int
	Header http.Header
	Raw    []byte
	Obj    map[string]any
	List   []map[string]any
}

func (r resp) str(k string) string {
	if s, ok := r.Obj[k].(string); ok {
		return s
	}
	return ""
}

// problem is the message of a field in a 422 answer.
func (r resp) problem(field string) string {
	d, _ := r.Obj["details"].(map[string]any)
	ps, _ := d["problems"].([]any)
	for _, p := range ps {
		m := p.(map[string]any)
		if m["field"] == field {
			return m["message"].(string)
		}
	}
	return ""
}

func call(t testing.TB, method, path string, body any, hdr ...string) resp {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(method, baseURL+path, rd)
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := client.Do(req)
	if err != nil {
		t.Errorf("%s %s: %v", method, path, err)
		return resp{}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Header: res.Header, Raw: raw}
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &out.List)
	} else {
		_ = json.Unmarshal(raw, &out.Obj)
	}
	return out
}

func want(t testing.TB, r resp, status int) bool {
	t.Helper()
	if r.Status != status {
		t.Errorf("status %d, want %d: %s", r.Status, status, trunc(r.Raw))
		return false
	}
	return true
}

func trunc(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "…"
	}
	return string(b)
}

/* ---------------- Load helpers ---------------- */

func parallel(n, workers int, fn func(i int)) {
	var wg sync.WaitGroup
	jobs := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(i)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}
