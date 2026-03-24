-- Migration 000006 rollback
DROP TABLE IF EXISTS knowledge_digest_log;
DROP TABLE IF EXISTS artifact_digest_sources;

ALTER TABLE knowledge_artifacts
    DROP CONSTRAINT knowledge_artifacts_knowledge_type_check;
ALTER TABLE knowledge_artifacts
    ADD CONSTRAINT knowledge_artifacts_knowledge_type_check
    CHECK (knowledge_type IN ('semantic', 'episodic', 'procedural', 'reference'));
