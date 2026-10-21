package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationAdvisoryLockKey(t *testing.T) {
	if MigrationAdvisoryLockKey == 0 {
		t.Fatal("MigrationAdvisoryLockKey must be a non-zero int64")
	}
	// Verify it matches "EVERROOT" ASCII hex (0x45564552524f4f54)
	expected := int64(0x45564552524f4f54)
	if MigrationAdvisoryLockKey != expected {
		t.Fatalf("expected lock key %x, got %x", expected, MigrationAdvisoryLockKey)
	}
}

func TestResolveMigrationDir(t *testing.T) {
	// From repository root
	dir, err := resolveMigrationDir("")
	if err != nil {
		t.Fatalf("expected to resolve db/migrations from repo root, got err: %v", err)
	}
	if !strings.HasSuffix(filepath.Clean(dir), "db/migrations") {
		t.Fatalf("expected resolved path to end with db/migrations, got: %s", dir)
	}

	// Non-existent directory fallback failure
	_, err = resolveMigrationDir("non_existent_directory_xyz_123")
	// If run from repo root, fallback to db/migrations will succeed because it's in candidates list,
	// but an isolated test with invalid path only:
	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "empty")
	_ = os.Mkdir(emptyDir, 0o755)
	resolved, err := resolveMigrationDir(emptyDir)
	if err != nil {
		t.Fatalf("expected preferred directory to resolve, got: %v", err)
	}
	if resolved != emptyDir {
		t.Fatalf("expected %s, got %s", emptyDir, resolved)
	}
}

func TestFindMigrationFiles(t *testing.T) {
	dir, err := resolveMigrationDir("")
	if err != nil {
		t.Fatalf("resolveMigrationDir failed: %v", err)
	}

	files, err := findMigrationFiles(dir)
	if err != nil {
		t.Fatalf("findMigrationFiles failed: %v", err)
	}

	if len(files) < 12 {
		t.Fatalf("expected at least 12 migration files, found %d", len(files))
	}

	// Verify ordering: 0001_registry.sql should be first, 0012_botanical_reference_catalogue.sql should be twelfth
	if files[0] != "0001_registry.sql" {
		t.Errorf("expected first migration to be 0001_registry.sql, got %s", files[0])
	}
	if files[11] != "0012_botanical_reference_catalogue.sql" {
		t.Errorf("expected twelfth migration to be 0012_botanical_reference_catalogue.sql, got %s", files[11])
	}

	// Verify all files are in strictly increasing order
	for i := 1; i < len(files); i++ {
		if files[i] <= files[i-1] {
			t.Errorf("migration files not strictly sorted: %s came after %s", files[i], files[i-1])
		}
	}
}

func TestMigrationFilesNonDestructive(t *testing.T) {
	dir, err := resolveMigrationDir("")
	if err != nil {
		t.Fatalf("resolveMigrationDir failed: %v", err)
	}

	files, err := findMigrationFiles(dir)
	if err != nil {
		t.Fatalf("findMigrationFiles failed: %v", err)
	}

	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}
		upper := strings.ToUpper(string(content))

		if strings.Contains(upper, "DROP TABLE") {
			t.Errorf("migration %s contains forbidden DROP TABLE statement", file)
		}
		if strings.Contains(upper, "TRUNCATE ") {
			t.Errorf("migration %s contains forbidden TRUNCATE statement", file)
		}
		if strings.Contains(upper, "DROP DATABASE") {
			t.Errorf("migration %s contains forbidden DROP DATABASE statement", file)
		}
	}
}

func TestFindMigrationFilesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := findMigrationFiles(tmpDir)
	if err == nil {
		t.Fatal("expected error on empty migration directory, got nil")
	}
}

func TestIsLegacyCatalogueMigration(t *testing.T) {
	// The catalogue and the gate around it: skippable, because discovery no
	// longer reads them.
	legacyFiles := []string{
		"0001_registry.sql",
		"0002_nakshatra_reference.sql",
		"0004_nakshatra_tradition_catalog.sql",
		"0012_botanical_reference_catalogue.sql",
	}
	for _, f := range legacyFiles {
		if !isLegacyCatalogueMigration(f) {
			t.Errorf("expected %s to be classified as legacy catalogue migration", f)
		}
	}

	// What a visitor writes. These must survive the default run, and getting
	// this boundary wrong is not academic: 0013 makes user_feedback carry
	// interaction_id, which POST /api/v1/feedback inserts into, and 0011 makes
	// discovery_sessions, which the conversation routes read. Both were once
	// filed under "legacy catalogue" and left a database that verified and then
	// failed at runtime.
	userDataFiles := []string{
		"0011_conversation_sessions.sql",
		"0013_user_interactions_and_feedback.sql",
		"0014_discovery_user_data.sql",
		"0015_user_preferences.sql",
	}
	for _, f := range userDataFiles {
		if isLegacyCatalogueMigration(f) {
			t.Errorf("expected %s to NOT be classified as legacy catalogue migration", f)
		}
	}
}
