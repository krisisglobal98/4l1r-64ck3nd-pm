-- The employee's email: where Authentication sends what belongs to the person (forgot password, reset links,
-- account notices) for the account linked to the employee; later the ESS profile. One live employee per address.
ALTER TABLE pm.employee ADD COLUMN email varchar(254) NULL;
ALTER TABLE pm.employee ADD CONSTRAINT employee_email_check CHECK (email IS NULL OR email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$');
CREATE UNIQUE INDEX employee_email_live ON pm.employee (lower(email)) WHERE NOT is_deleted AND email IS NOT NULL;
