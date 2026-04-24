-- Migration 000006 rollback
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_digest_log;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.artifact_digest_sources;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP CONSTRAINT knowledge_artifacts_knowledge_type_check;
ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    ADD CONSTRAINT knowledge_artifacts_knowledge_type_check
    CHECK (knowledge_type IN ('semantic', 'episodic', 'procedural', 'reference'));
