DROP INDEX pm.org_level_rank_live;
CREATE INDEX idx_org_level_rank ON pm.org_level (rank);
