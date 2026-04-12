DROP TRIGGER IF EXISTS trigger_search_index_update ON resources;
DROP FUNCTION IF EXISTS fn_update_search_index();

DROP TABLE IF EXISTS anomaly_flags;
DROP TABLE IF EXISTS search_index;
DROP TABLE IF EXISTS user_search_history;
DROP TABLE IF EXISTS search_terms;
DROP TABLE IF EXISTS synonym_groups;
DROP TABLE IF EXISTS pinyin_mapping;
DROP TABLE IF EXISTS recommendation_strategy_config;
DROP TABLE IF EXISTS ranking_archives;
DROP TABLE IF EXISTS user_badges;
DROP TABLE IF EXISTS badges;
DROP TABLE IF EXISTS point_rules;
DROP TABLE IF EXISTS point_transactions;
DROP TABLE IF EXISTS user_points;
DROP TABLE IF EXISTS follows;
DROP TABLE IF EXISTS favorites;
DROP TABLE IF EXISTS votes;
