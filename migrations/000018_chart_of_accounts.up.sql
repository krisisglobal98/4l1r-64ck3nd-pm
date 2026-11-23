-- Chart of Accounts (Bagan Akun, Keuangan & Pajak): the accounts the Finance module will journal to, kept in Portal
-- Master like every master. Shaped as accounting applications (Accurate, Zahir) do: an account number of the
-- company's own, a name, an account type (which fixes its normal balance), header accounts grouping detail accounts
-- of the same type, and an optional currency (a foreign-currency bank, receivable or payable).
-- Kode Biaya now points to two accounts: the revenue account when billed to a customer and the cost account when paid
-- to a vendor (the free-text account_code it had until now goes).
CREATE TABLE pm.account (
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,
    valid_from              date         NULL,
    valid_to                date         NULL,
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,
    -- Account Attributes
    account_no              varchar(30)  NOT NULL,      -- the company's account number, e.g. 4-1100 or 410100
    account_type            varchar(30)  NOT NULL,      -- cash_bank … other_expense (see the check)
    normal_balance          varchar(6)   NOT NULL,      -- debit or credit, from the type
    is_header               boolean      NOT NULL DEFAULT false,  -- groups detail accounts; never journaled to
    parent_id               uuid         NULL,          -- its header account (same type)
    currency_id             uuid         NULL,          -- only for a foreign-currency account
    CONSTRAINT account_pkey PRIMARY KEY (id),
    CONSTRAINT account_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT account_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT account_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT account_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT account_delete_audit_check CHECK (is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)),
    CONSTRAINT account_no_check CHECK (account_no ~ '^[0-9A-Za-z][0-9A-Za-z.\-]*$'),
    CONSTRAINT account_type_check CHECK (account_type IN ('cash_bank', 'receivable', 'inventory', 'other_current_asset', 'fixed_asset',
        'accumulated_depreciation', 'other_asset', 'payable', 'other_current_liability', 'long_term_liability', 'equity', 'revenue', 'cogs',
        'expense', 'other_income', 'other_expense')),
    CONSTRAINT account_balance_check CHECK (normal_balance IN ('debit', 'credit')),
    CONSTRAINT account_parent_fk FOREIGN KEY (parent_id) REFERENCES pm.account (id),
    CONSTRAINT account_currency_fk FOREIGN KEY (currency_id) REFERENCES pm.currency (id)
);
CREATE UNIQUE INDEX account_code_ci ON pm.account (tenant_id, lower(code));
CREATE UNIQUE INDEX account_no_live ON pm.account (tenant_id, lower(account_no)) WHERE NOT is_deleted;
CREATE INDEX idx_account_active ON pm.account (tenant_id, is_active, is_deleted);
CREATE INDEX idx_account_parent ON pm.account (parent_id);
ALTER TABLE pm.account ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.account USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

ALTER TABLE pm.charge_code
    ADD COLUMN revenue_account_id uuid NULL,   -- when billed to a customer
    ADD COLUMN cost_account_id    uuid NULL,   -- when paid to a vendor
    ADD CONSTRAINT charge_code_revenue_account_fk FOREIGN KEY (revenue_account_id) REFERENCES pm.account (id),
    ADD CONSTRAINT charge_code_cost_account_fk FOREIGN KEY (cost_account_id) REFERENCES pm.account (id),
    DROP COLUMN account_code;
