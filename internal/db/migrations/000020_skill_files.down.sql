-- Migration 000020 rollback: drop multi-file skill tables
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.skill_history_files;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.skill_files;
