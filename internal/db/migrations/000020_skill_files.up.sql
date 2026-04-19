-- Migration 000020: multi-file skill support
-- Adds skill_files for supplementary files per skill, and skill_history_files
-- to snapshot file content alongside the existing skill_history version records.

-- ─────────────────────────────────────────
-- 1. Supplementary skill files
-- ─────────────────────────────────────────
CREATE TABLE skill_files (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    skill_id      UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    -- relative_path includes the typed subdirectory prefix:
    --   scripts/foo.sh   → executable script
    --   references/bar.md → additional markdown reference
    relative_path TEXT NOT NULL,
    content       TEXT NOT NULL,
    is_executable BOOL NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_id, relative_path)
);

CREATE INDEX skill_files_skill_id_idx ON skill_files (skill_id);

CREATE TRIGGER skill_files_updated_at BEFORE UPDATE ON skill_files
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ─────────────────────────────────────────
-- 2. File history snapshots
-- Correlates with skill_history via (skill_id, version).
-- ─────────────────────────────────────────
CREATE TABLE skill_history_files (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    skill_id      UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version       INT  NOT NULL,
    relative_path TEXT NOT NULL,
    content       TEXT NOT NULL,
    is_executable BOOL NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX skill_history_files_skill_version_idx
    ON skill_history_files (skill_id, version);
