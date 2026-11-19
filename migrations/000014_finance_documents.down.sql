DELETE FROM pm.code_prefix WHERE code IN ('exchange_rate', 'charge_code', 'company_bank_account', 'document_type', 'service_type');
DROP TABLE pm.service_type;
DROP TABLE pm.document_type;
DROP TABLE pm.company_bank_account;
DROP TABLE pm.charge_code_rate;
DROP TABLE pm.charge_code;
DROP TABLE pm.exchange_rate;
