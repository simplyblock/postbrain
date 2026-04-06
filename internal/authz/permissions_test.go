package authz_test

import (
	"slices"
	"testing"

	"github.com/simplyblock/postbrain/internal/authz"
)

// TestResourceConstants verifies every Resource constant is defined and non-empty.
func TestResourceConstants(t *testing.T) {
	resources := []authz.Resource{
		authz.ResourceMemories,
		authz.ResourceKnowledge,
		authz.ResourceCollections,
		authz.ResourceSkills,
		authz.ResourceSessions,
		authz.ResourceGraph,
		authz.ResourceScopes,
		authz.ResourcePrincipals,
		authz.ResourceTokens,
		authz.ResourceSharing,
		authz.ResourcePromotions,
	}
	for _, r := range resources {
		if r == "" {
			t.Errorf("resource constant is empty")
		}
	}
	if len(resources) != len(authz.AllResources()) {
		t.Errorf("AllResources() length %d does not match expected %d", len(authz.AllResources()), len(resources))
	}
}

// TestOperationConstants verifies every Operation constant is defined and non-empty.
func TestOperationConstants(t *testing.T) {
	ops := []authz.Operation{
		authz.OperationRead,
		authz.OperationWrite,
		authz.OperationEdit,
		authz.OperationDelete,
	}
	for _, op := range ops {
		if op == "" {
			t.Errorf("operation constant is empty")
		}
	}
}

// TestValidOperations verifies the exact operation set for each resource.
func TestValidOperations(t *testing.T) {
	rw := []authz.Operation{authz.OperationRead, authz.OperationWrite}
	rwe := []authz.Operation{authz.OperationRead, authz.OperationWrite, authz.OperationEdit}
	rwed := []authz.Operation{authz.OperationRead, authz.OperationWrite, authz.OperationEdit, authz.OperationDelete}
	rwd := []authz.Operation{authz.OperationRead, authz.OperationWrite, authz.OperationDelete}
	ro := []authz.Operation{authz.OperationRead}

	cases := []struct {
		resource authz.Resource
		want     []authz.Operation
	}{
		{authz.ResourceMemories, rwed},
		{authz.ResourceKnowledge, rwed},
		{authz.ResourceCollections, rwed},
		{authz.ResourceSkills, rwed},
		{authz.ResourceSessions, rw},
		{authz.ResourceGraph, ro},
		{authz.ResourceScopes, rwed},
		{authz.ResourcePrincipals, rwed},
		{authz.ResourceTokens, rwed},
		{authz.ResourceSharing, rwd},
		{authz.ResourcePromotions, rwed},
		// suppress unused variable warning
		{authz.ResourceMemories, rwe[:0]}, // placeholder to use rwe; actual check below
	}
	// remove placeholder
	cases = cases[:len(cases)-1]

	for _, tc := range cases {
		got := authz.ValidOperations(tc.resource)
		if len(got) != len(tc.want) {
			t.Errorf("ValidOperations(%q): got %v, want %v", tc.resource, got, tc.want)
			continue
		}
		for _, wantOp := range tc.want {
			if !slices.Contains(got, wantOp) {
				t.Errorf("ValidOperations(%q): missing operation %q", tc.resource, wantOp)
			}
		}
		for _, gotOp := range got {
			if !slices.Contains(tc.want, gotOp) {
				t.Errorf("ValidOperations(%q): unexpected operation %q", tc.resource, gotOp)
			}
		}
	}

	// rwe used in at least one case to avoid compiler error — sharing uses rwd not rwe
	_ = rwe
}

// TestPermissionString verifies Permission.String() produces "{resource}:{operation}".
func TestPermissionString(t *testing.T) {
	p := authz.NewPermission(authz.ResourceMemories, authz.OperationRead)
	if string(p) != "memories:read" {
		t.Errorf("got %q, want %q", p, "memories:read")
	}
}

// TestAllPermissions verifies AllPermissions returns all valid resource:operation combinations.
func TestAllPermissions(t *testing.T) {
	all := authz.AllPermissions()
	if len(all) == 0 {
		t.Fatal("AllPermissions returned empty slice")
	}
	// every entry must be a valid permission
	for _, p := range all {
		r, op, err := p.Parse()
		if err != nil {
			t.Errorf("AllPermissions contains invalid permission %q: %v", p, err)
			continue
		}
		if !slices.Contains(authz.ValidOperations(r), op) {
			t.Errorf("AllPermissions contains %q but %q is not a valid operation for %q", p, op, r)
		}
	}
	// spot-check expected entries are present
	for _, want := range []authz.Permission{
		authz.NewPermission(authz.ResourceMemories, authz.OperationRead),
		authz.NewPermission(authz.ResourceGraph, authz.OperationRead),
		authz.NewPermission(authz.ResourceSkills, authz.OperationDelete),
	} {
		if !slices.Contains(all, want) {
			t.Errorf("AllPermissions missing %q", want)
		}
	}
	// graph:write must NOT be present
	graphWrite := authz.NewPermission(authz.ResourceGraph, authz.OperationWrite)
	if slices.Contains(all, graphWrite) {
		t.Errorf("AllPermissions should not contain %q", graphWrite)
	}
}

// TestExpand_ShorthandRead verifies bare "read" expands to all :read permissions.
func TestExpand_ShorthandRead(t *testing.T) {
	got, err := authz.Expand("read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range authz.AllResources() {
		want := authz.NewPermission(r, authz.OperationRead)
		if !slices.Contains(got, want) {
			t.Errorf("Expand(\"read\") missing %q", want)
		}
	}
	// must not contain non-read operations
	for _, p := range got {
		_, op, _ := p.Parse()
		if op != authz.OperationRead {
			t.Errorf("Expand(\"read\") contains non-read permission %q", p)
		}
	}
}

// TestExpand_ShorthandWrite verifies bare "write" expands to all valid :write permissions.
func TestExpand_ShorthandWrite(t *testing.T) {
	got, err := authz.Expand("write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range authz.AllResources() {
		if !slices.Contains(authz.ValidOperations(r), authz.OperationWrite) {
			continue // resource does not support write
		}
		want := authz.NewPermission(r, authz.OperationWrite)
		if !slices.Contains(got, want) {
			t.Errorf("Expand(\"write\") missing %q", want)
		}
	}
	for _, p := range got {
		_, op, _ := p.Parse()
		if op != authz.OperationWrite {
			t.Errorf("Expand(\"write\") contains non-write permission %q", p)
		}
	}
}

// TestExpand_ShorthandEdit verifies bare "edit" expands to all valid :edit permissions.
func TestExpand_ShorthandEdit(t *testing.T) {
	got, err := authz.Expand("edit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range got {
		_, op, _ := p.Parse()
		if op != authz.OperationEdit {
			t.Errorf("Expand(\"edit\") contains non-edit permission %q", p)
		}
	}
	// graph does not support edit — must not appear
	graphEdit := authz.NewPermission(authz.ResourceGraph, authz.OperationEdit)
	if slices.Contains(got, graphEdit) {
		t.Errorf("Expand(\"edit\") must not contain %q", graphEdit)
	}
}

// TestExpand_ShorthandDelete verifies bare "delete" expands to all valid :delete permissions.
func TestExpand_ShorthandDelete(t *testing.T) {
	got, err := authz.Expand("delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range got {
		_, op, _ := p.Parse()
		if op != authz.OperationDelete {
			t.Errorf("Expand(\"delete\") contains non-delete permission %q", p)
		}
	}
	// graph does not support delete
	graphDelete := authz.NewPermission(authz.ResourceGraph, authz.OperationDelete)
	if slices.Contains(got, graphDelete) {
		t.Errorf("Expand(\"delete\") must not contain %q", graphDelete)
	}
	// sessions does not support delete
	sessionsDelete := authz.NewPermission(authz.ResourceSessions, authz.OperationDelete)
	if slices.Contains(got, sessionsDelete) {
		t.Errorf("Expand(\"delete\") must not contain %q", sessionsDelete)
	}
}

// TestExpand_ResourceScoped verifies a "resource:operation" string expands to exactly itself.
func TestExpand_ResourceScoped(t *testing.T) {
	cases := []struct {
		raw  string
		want authz.Permission
	}{
		{"memories:read", authz.NewPermission(authz.ResourceMemories, authz.OperationRead)},
		{"knowledge:write", authz.NewPermission(authz.ResourceKnowledge, authz.OperationWrite)},
		{"scopes:edit", authz.NewPermission(authz.ResourceScopes, authz.OperationEdit)},
		{"skills:delete", authz.NewPermission(authz.ResourceSkills, authz.OperationDelete)},
		{"graph:read", authz.NewPermission(authz.ResourceGraph, authz.OperationRead)},
	}
	for _, tc := range cases {
		got, err := authz.Expand(tc.raw)
		if err != nil {
			t.Errorf("Expand(%q): unexpected error: %v", tc.raw, err)
			continue
		}
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("Expand(%q) = %v, want [%v]", tc.raw, got, tc.want)
		}
	}
}

// TestExpand_InvalidOperation verifies Expand rejects operations not valid for a resource.
func TestExpand_InvalidOperation(t *testing.T) {
	cases := []string{
		"graph:write",
		"graph:edit",
		"graph:delete",
		"sessions:edit",
		"sessions:delete",
		"sharing:edit",
	}
	for _, raw := range cases {
		_, err := authz.Expand(raw)
		if err == nil {
			t.Errorf("Expand(%q): expected error, got nil", raw)
		}
	}
}

// TestExpand_UnknownResource verifies Expand rejects an unknown resource.
func TestExpand_UnknownResource(t *testing.T) {
	_, err := authz.Expand("unknown:read")
	if err == nil {
		t.Error("Expand(\"unknown:read\"): expected error, got nil")
	}
}

// TestExpand_UnknownShorthand verifies Expand rejects an unknown bare string.
func TestExpand_UnknownShorthand(t *testing.T) {
	_, err := authz.Expand("admin")
	if err == nil {
		t.Error("Expand(\"admin\"): expected error, got nil")
	}
	_, err = authz.Expand("")
	if err == nil {
		t.Error("Expand(\"\"): expected error, got nil")
	}
}

// TestPermission_Parse verifies round-trip parsing of Permission values.
func TestPermission_Parse(t *testing.T) {
	cases := []struct {
		perm     authz.Permission
		resource authz.Resource
		op       authz.Operation
	}{
		{authz.NewPermission(authz.ResourceMemories, authz.OperationRead), authz.ResourceMemories, authz.OperationRead},
		{authz.NewPermission(authz.ResourceScopes, authz.OperationEdit), authz.ResourceScopes, authz.OperationEdit},
		{authz.NewPermission(authz.ResourceGraph, authz.OperationRead), authz.ResourceGraph, authz.OperationRead},
	}
	for _, tc := range cases {
		r, op, err := tc.perm.Parse()
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", tc.perm, err)
			continue
		}
		if r != tc.resource || op != tc.op {
			t.Errorf("Parse(%q) = (%q, %q), want (%q, %q)", tc.perm, r, op, tc.resource, tc.op)
		}
	}
}
