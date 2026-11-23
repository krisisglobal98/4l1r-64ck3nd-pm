// Command pm-service is the Portal Master backend of ALIR.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alir.local/pm-service/internal/approvalrule"
	"alir.local/pm-service/internal/bank"
	"alir.local/pm-service/internal/chargecode"
	"alir.local/pm-service/internal/city"
	"alir.local/pm-service/internal/commodity"
	"alir.local/pm-service/internal/companyaccount"
	"alir.local/pm-service/internal/coa"
	"alir.local/pm-service/internal/companyprofile"
	"alir.local/pm-service/internal/containertype"
	"alir.local/pm-service/internal/country"
	"alir.local/pm-service/internal/currency"
	"alir.local/pm-service/internal/customflow"
	"alir.local/pm-service/internal/delegation"
	"alir.local/pm-service/internal/documenttype"
	"alir.local/pm-service/internal/educationlevel"
	"alir.local/pm-service/internal/employee"
	"alir.local/pm-service/internal/employmentstatus"
	"alir.local/pm-service/internal/exchangerate"
	"alir.local/pm-service/internal/grade"
	"alir.local/pm-service/internal/incoterm"
	"alir.local/pm-service/internal/jabatan"
	"alir.local/pm-service/internal/location"
	"alir.local/pm-service/internal/numbering"
	"alir.local/pm-service/internal/orglevel"
	"alir.local/pm-service/internal/orgunit"
	"alir.local/pm-service/internal/packaging"
	"alir.local/pm-service/internal/partner"
	"alir.local/pm-service/internal/paymentterm"
	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/config"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/httpx/reqid"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/tenant"
	"alir.local/pm-service/internal/position"
	"alir.local/pm-service/internal/salesperson"
	"alir.local/pm-service/internal/servicetype"
	"alir.local/pm-service/internal/sysparam"
	"alir.local/pm-service/internal/taxtype"
	"alir.local/pm-service/internal/unittype"
	"alir.local/pm-service/internal/uom"
	"alir.local/pm-service/internal/workshift"
)

func main() {
	if err := run(); err != nil {
		slog.Error("pm-service stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Pool.Close()
	if err := db.RequireRLS(ctx); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = httpx.JSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Pool.Ping(r.Context()); err != nil {
			_ = httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
			return
		}
		_ = httpx.JSON(w, 200, map[string]string{"status": "ready"})
	})

	codes, events := codeprefix.Postgres{DB: db}, outbox.Writer{DB: db}
	orglevel.Routes(mux, &orglevel.Service{Repo: orglevel.PostgresRepo{DB: db}, Tx: db, Codes: codes, Events: events})
	unittype.Routes(mux, &unittype.Service{Repo: unittype.PostgresRepo{DB: db}, Tx: db, Codes: codes, Events: events})
	orgunit.Routes(mux, &orgunit.Service{Repo: orgunit.PostgresRepo{DB: db}, Tx: db, Codes: codes, Events: events})
	master.Routes(mux, "/pm/masters/grades", grade.New(db))
	master.Routes(mux, "/pm/masters/jabatan", jabatan.New(db))
	master.Routes(mux, "/pm/masters/positions", position.New(db))
	master.Routes(mux, "/pm/masters/approval-rules", approvalrule.New(db))
	master.Routes(mux, "/pm/masters/delegations", delegation.New(db))
	master.Routes(mux, "/pm/masters/custom-flows", customflow.New(db))
	master.Routes(mux, "/pm/masters/employment-status", employmentstatus.New(db))
	master.Routes(mux, "/pm/masters/education-levels", educationlevel.New(db))
	master.Routes(mux, "/pm/masters/work-shifts", workshift.New(db))
	master.Routes(mux, "/pm/masters/banks", bank.New(db))
	master.Routes(mux, "/pm/masters/company-profiles", companyprofile.New(db))
	companyprofile.PrintRoutes(mux, db)
	master.Routes(mux, "/pm/masters/system-parameters", sysparam.New(db))
	sysparam.ReadRoutes(mux, db)
	master.Routes(mux, "/pm/masters/accounts", coa.New(db))
	// Keuangan & Pajak, Geografi & Lokasi (reference masters every module shares)
	master.Routes(mux, "/pm/masters/currencies", currency.New(db))
	master.Routes(mux, "/pm/masters/countries", country.New(db))
	master.Routes(mux, "/pm/masters/cities", city.New(db))
	master.Routes(mux, "/pm/masters/payment-terms", paymentterm.New(db))
	master.Routes(mux, "/pm/masters/tax-types", taxtype.New(db))
	// Mitra Bisnis
	master.Routes(mux, "/pm/masters/sales-persons", salesperson.New(db))
	master.Routes(mux, "/pm/masters/business-partners", partner.New(db))
	// Logistik & Pengiriman, and the ports of Geografi & Lokasi
	master.Routes(mux, "/pm/masters/ports", location.New(db))
	master.Routes(mux, "/pm/masters/incoterms", incoterm.New(db))
	master.Routes(mux, "/pm/masters/containers", containertype.New(db))
	master.Routes(mux, "/pm/masters/packagings", packaging.New(db))
	master.Routes(mux, "/pm/masters/uoms", uom.New(db))
	master.Routes(mux, "/pm/masters/commodities", commodity.New(db))
	master.Routes(mux, "/pm/masters/service-types", servicetype.New(db))
	// Keuangan & Pajak: rates, charges, the company's accounts; Dokumen
	exchangerate.Routes(mux, "/pm/masters/exchange-rates", db)
	// Pengaturan Sistem
	numbering.Routes(mux, "/pm/masters/numbering", db)
	master.Routes(mux, "/pm/masters/charges", chargecode.New(db))
	master.Routes(mux, "/pm/masters/company-bank-accounts", companyaccount.New(db))
	master.Routes(mux, "/pm/masters/document-types", documenttype.New(db))
	master.Routes(mux, "/pm/masters/employees", employee.New(db))

	// Who calls: the access token of the Authentication service; a default user only when chosen explicitly.
	caller := (&access.JWT{JWKSURL: cfg.JWKSURL, Issuer: cfg.AuthIssuer, Client: &http.Client{Timeout: 5 * time.Second}}).Middleware
	if cfg.AuthMode == "default-user" {
		slog.Warn("PM_AUTH_MODE=default-user: every request has full access (development only)")
		caller = access.DefaultUser(cfg.DefaultUserID, cfg.DefaultUser, cfg.DefaultTenantID)
	}
	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpx.Chain(mux, reqid.Middleware, httpx.Recover, httpx.Logging, httpx.JSONErrors, caller,
			db.TenantScope(access.TenantOf), (&tenant.Provisioner{DB: db}).Middleware),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("pm-service listening", "port", cfg.Port, "auth_mode", cfg.AuthMode, "jwks", cfg.JWKSURL)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shut)
	}
	return nil
}
