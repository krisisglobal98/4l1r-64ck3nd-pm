package master

import (
	"context"
	"errors"
	"testing"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/record"
)

// A global master (countries, currencies, Incoterms) is changed by the platform administrator only, even by a
// company's admin with full access, and never deleted.
func TestGlobalMasterGuard(t *testing.T) {
	s := &Service[struct{}, struct{}]{Spec: Spec[struct{}, struct{}]{Menu: "pm.currency", Table: "currency", Global: true}}
	admin := access.WithUser(context.Background(), access.User{ID: "u", TenantID: "t", Full: true})
	platform := access.WithUser(context.Background(), access.User{ID: "p", TenantID: "t", Full: true, Platform: true})
	var e *apperr.Error
	if _, err := s.Create(admin, record.Values{}); !errors.As(err, &e) || e.Code != "GLOBAL_MASTER" || e.Status != 403 {
		t.Errorf("create by a company admin: %v", err)
	}
	if _, err := s.Update(admin, "MMU001", record.Values{}); !errors.As(err, &e) || e.Code != "GLOBAL_MASTER" {
		t.Errorf("update by a company admin: %v", err)
	}
	for _, ctx := range []context.Context{admin, platform} {
		if err := s.Delete(ctx, "MMU001"); !errors.As(err, &e) || e.Code != "GLOBAL_MASTER" || e.Status != 409 {
			t.Errorf("delete: %v", err)
		}
	}
	if err := s.writable(access.User{Platform: true}); err != nil {
		t.Errorf("the platform administrator writes: %v", err)
	}
	s.Spec.Global = false
	if err := s.writable(access.User{Full: true}); err != nil {
		t.Errorf("a tenant master: %v", err)
	}
}
