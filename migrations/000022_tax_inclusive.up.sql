-- PPN whose price already holds the tax ("harga sudah termasuk pajak", LKT categories Ppn2 and a NON PPN with a rate):
-- the module takes it out of the line amount (amount × rate ÷ (100 + rate)) instead of adding it on top.
ALTER TABLE pm.tax_type ADD COLUMN is_inclusive boolean NOT NULL DEFAULT false;
ALTER TABLE pm.tax_type ADD CONSTRAINT tax_type_inclusive_check CHECK (NOT is_inclusive OR method = 'added');
