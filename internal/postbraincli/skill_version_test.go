package postbraincli

import "testing"

func TestReadSkillVersion_ParsesFrontmatter(t *testing.T) {
	t.Parallel()
	content := "---\nversion: 3\n---\n\n# Title\n"
	if got := ReadSkillVersion(content); got != 3 {
		t.Fatalf("ReadSkillVersion = %d, want 3", got)
	}
}

func TestReadSkillVersion_ReturnsZeroWhenNoFrontmatter(t *testing.T) {
	t.Parallel()
	content := "# Title\nNo frontmatter here.\n"
	if got := ReadSkillVersion(content); got != 0 {
		t.Fatalf("ReadSkillVersion = %d, want 0", got)
	}
}

func TestReadSkillVersion_ReturnsZeroWhenVersionMissing(t *testing.T) {
	t.Parallel()
	content := "---\nname: foo\n---\n\n# Title\n"
	if got := ReadSkillVersion(content); got != 0 {
		t.Fatalf("ReadSkillVersion = %d, want 0", got)
	}
}

func TestReadSkillVersion_ReturnsZeroOnEmpty(t *testing.T) {
	t.Parallel()
	if got := ReadSkillVersion(""); got != 0 {
		t.Fatalf("ReadSkillVersion = %d, want 0", got)
	}
}