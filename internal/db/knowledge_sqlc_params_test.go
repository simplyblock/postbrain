package db

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestCreateArtifactParams_UsesTypedVersionAndSourceIDs(t *testing.T) {
	t.Helper()

	typ := reflect.TypeOf(CreateArtifactParams{})

	previousVersion, ok := typ.FieldByName("PreviousVersion")
	if !ok {
		t.Fatal("CreateArtifactParams.PreviousVersion field missing")
	}
	if previousVersion.Type != reflect.TypeOf((*uuid.UUID)(nil)) {
		t.Fatalf("CreateArtifactParams.PreviousVersion has type %v, want *uuid.UUID", previousVersion.Type)
	}

	sourceMemoryID, ok := typ.FieldByName("SourceMemoryID")
	if !ok {
		t.Fatal("CreateArtifactParams.SourceMemoryID field missing")
	}
	if sourceMemoryID.Type != reflect.TypeOf((*uuid.UUID)(nil)) {
		t.Fatalf("CreateArtifactParams.SourceMemoryID has type %v, want *uuid.UUID", sourceMemoryID.Type)
	}

	if _, hasColumn17 := typ.FieldByName("Column17"); hasColumn17 {
		t.Fatal("CreateArtifactParams unexpectedly contains Column17")
	}
	if _, hasColumn18 := typ.FieldByName("Column18"); hasColumn18 {
		t.Fatal("CreateArtifactParams unexpectedly contains Column18")
	}
}
