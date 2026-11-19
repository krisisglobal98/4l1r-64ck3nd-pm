-- One live level per rank: rank orders the organization tree (1 = top) and "one level above" is rank - 1, so two
-- live levels on one rank make the tree ambiguous. Deleted levels free their rank.
CREATE UNIQUE INDEX org_level_rank_live ON pm.org_level (rank) WHERE NOT is_deleted;
DROP INDEX pm.idx_org_level_rank;
