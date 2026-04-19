package skills

import (
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
)

func TestValidateSkillFile(t *testing.T) {
	tests := []struct {
		name    string
		input   db.SkillFileInput
		wantErr string // substring; empty = no error expected
	}{
		// ── valid cases ──────────────────────────────────────────────────────
		{
			name:    "valid script",
			input:   db.SkillFileInput{RelativePath: "scripts/run.sh", Content: "#!/bin/sh", IsExecutable: true},
			wantErr: "",
		},
		{
			name:    "valid reference",
			input:   db.SkillFileInput{RelativePath: "references/guide.md", Content: "# Guide", IsExecutable: false},
			wantErr: "",
		},
		{
			name:    "valid nested script",
			input:   db.SkillFileInput{RelativePath: "scripts/sub/helper.py", Content: "print('hi')", IsExecutable: true},
			wantErr: "",
		},

		// ── empty / reserved ─────────────────────────────────────────────────
		{
			name:    "empty path",
			input:   db.SkillFileInput{RelativePath: "", Content: "x"},
			wantErr: "empty",
		},
		{
			name:    "reserved SKILL.md",
			input:   db.SkillFileInput{RelativePath: "SKILL.md", Content: "x"},
			wantErr: "reserved",
		},

		// ── absolute paths ───────────────────────────────────────────────────
		{
			name:    "absolute unix",
			input:   db.SkillFileInput{RelativePath: "/etc/passwd", Content: "x"},
			wantErr: "absolute",
		},
		{
			name:    "absolute backslash",
			input:   db.SkillFileInput{RelativePath: `\Windows\system32`, Content: "x"},
			wantErr: "absolute",
		},

		// ── traversal ────────────────────────────────────────────────────────
		{
			name:    "dotdot segment",
			input:   db.SkillFileInput{RelativePath: "scripts/../../../etc/passwd", Content: "x", IsExecutable: true},
			wantErr: "traversal",
		},
		{
			name:    "dotdot as entire path",
			input:   db.SkillFileInput{RelativePath: "..", Content: "x"},
			wantErr: "traversal",
		},

		// ── backslash in path ─────────────────────────────────────────────────
		{
			name:    "backslash in segment",
			input:   db.SkillFileInput{RelativePath: `scripts\run.sh`, Content: "x", IsExecutable: true},
			wantErr: "backslash",
		},

		// ── segment character rules ──────────────────────────────────────────
		{
			name:    "segment starting with dot",
			input:   db.SkillFileInput{RelativePath: "scripts/.hidden.sh", Content: "x", IsExecutable: true},
			wantErr: "segment",
		},
		{
			name:    "segment with space",
			input:   db.SkillFileInput{RelativePath: "scripts/my script.sh", Content: "x", IsExecutable: true},
			wantErr: "segment",
		},
		{
			name:    "empty segment (double slash)",
			input:   db.SkillFileInput{RelativePath: "scripts//run.sh", Content: "x", IsExecutable: true},
			wantErr: "segment",
		},

		// ── path too long ─────────────────────────────────────────────────────
		{
			name:    "path too long",
			input:   db.SkillFileInput{RelativePath: "scripts/" + strings.Repeat("a", 250) + ".sh", Content: "x", IsExecutable: true},
			wantErr: "too long",
		},

		// ── subdirectory prefix enforcement ──────────────────────────────────
		{
			name:    "executable not in scripts/",
			input:   db.SkillFileInput{RelativePath: "references/run.sh", Content: "x", IsExecutable: true},
			wantErr: "scripts/",
		},
		{
			name:    "markdown not in references/",
			input:   db.SkillFileInput{RelativePath: "scripts/guide.md", Content: "x", IsExecutable: false},
			wantErr: "references/",
		},
		{
			name:    "non-executable non-md in wrong place",
			input:   db.SkillFileInput{RelativePath: "scripts/config.json", Content: "x", IsExecutable: false},
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSkillFile(tc.input)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
			}
		})
	}
}
