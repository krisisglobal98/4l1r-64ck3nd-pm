//go:build e2e

// System Parameter (/pm/masters/system-parameters) and the read the modules use (/pm/parameters/{key}).
package e2e

import "testing"

const sysParamBase = "/pm/masters/system-parameters"

func TestSystemParameter(t *testing.T) {
	t.Run("SP01 a parameter by key; the modules read its value; inactive is not read", func(t *testing.T) {
		if x := call(t, "GET", "/pm/parameters/INVOICE_DEFAULT_DUE_DAYS", nil); x.Status != 404 {
			t.Fatalf("none yet: %d", x.Status)
		}
		p := must(t, call(t, "POST", sysParamBase, map[string]any{"name": "Jatuh tempo default invoice", "key": "invoice_default_due_days", "value": "14", "module": "freight-forwarding"}), 201)
		if !genCodeOf("MSP").MatchString(p.str("code")) || p.str("key") != "INVOICE_DEFAULT_DUE_DAYS" || p.str("module") != "freight-forwarding" {
			t.Errorf("created %s", trunc(p.Raw))
		}
		if x := must(t, call(t, "GET", "/pm/parameters/invoice_default_due_days", nil), 200); x.str("value") != "14" {
			t.Errorf("read %s", trunc(x.Raw))
		}
		must(t, call(t, "PUT", sysParamBase+"/"+p.str("code"), map[string]any{"name": "x", "key": "INVOICE_DEFAULT_DUE_DAYS", "value": "21", "active": false, "version": p.Obj["version"]}), 200)
		if x := call(t, "GET", "/pm/parameters/INVOICE_DEFAULT_DUE_DAYS", nil); x.Status != 404 {
			t.Errorf("inactive is not read: %d", x.Status)
		}
	})
	t.Run("SP02 bad keys and values, a key twice, a key changed", func(t *testing.T) {
		for field, b := range map[string]map[string]any{
			"key":   {"name": "x", "key": "1abc", "value": "1"},
			"value": {"name": "x", "key": "INVOICE_DEFAULT_DUE_DAYS", "value": "lima"},
		} {
			if x := call(t, "POST", sysParamBase, b); !want(t, x, 422) || x.problem(field) == "" {
				t.Errorf("%s: %s", field, trunc(x.Raw))
			}
		}
		a := must(t, call(t, "POST", sysParamBase, map[string]any{"name": "Contoh", "key": "CONTOH_A", "value": "a"}), 201)
		if x := call(t, "POST", sysParamBase, map[string]any{"name": "Contoh", "key": "CONTOH_A", "value": "b"}); !want(t, x, 422) || x.problem("key") == "" {
			t.Errorf("twice %s", trunc(x.Raw))
		}
		if x := call(t, "PUT", sysParamBase+"/"+a.str("code"), map[string]any{"name": "Contoh", "key": "CONTOH_B", "value": "a"}); !want(t, x, 422) || x.problem("key") == "" {
			t.Errorf("key changed %s", trunc(x.Raw))
		}
	})
}
