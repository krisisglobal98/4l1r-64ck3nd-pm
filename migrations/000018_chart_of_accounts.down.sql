ALTER TABLE pm.charge_code DROP COLUMN revenue_account_id, DROP COLUMN cost_account_id, ADD COLUMN account_code varchar(30) NULL;
DROP TABLE pm.account;
