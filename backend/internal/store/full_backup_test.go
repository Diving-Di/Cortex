package store

import "testing"

func TestFullBackupTableWhitelistExcludesSensitiveState(t *testing.T) {
	forbidden := map[string]bool{
		"ai_providers":       true,
		"ai_usage_records":   true,
		"audit_logs":         true,
		"auth_tokens":        true,
		"users":              true,
		"tenants":            true,
		"xhs_authorizations": true,
		"xhs_auth_attempts":  true,
	}
	seen := make(map[string]bool)
	for _, spec := range backupTableSpecs {
		if forbidden[spec.name] {
			t.Fatalf("sensitive table %q must not be exported", spec.name)
		}
		if seen[spec.name] {
			t.Fatalf("duplicate backup table %q", spec.name)
		}
		seen[spec.name] = true
	}
	for _, required := range []string{
		"notes", "note_revisions", "tags", "attachments",
		"knowledge_collections", "knowledge_documents",
		"research_sources", "research_drafts", "research_assets",
	} {
		if !seen[required] {
			t.Errorf("required table %q is missing", required)
		}
	}
}

func TestCloneBackupRowDoesNotMutateManifest(t *testing.T) {
	original := map[string]interface{}{"id": "old", "title": "entry"}
	clone := cloneBackupRow(original)
	delete(clone, "id")
	if original["id"] != "old" {
		t.Fatal("restore row transformation mutated the backup manifest")
	}
}

func TestBackupTableBytesSumsAndRejectsInvalidValues(t *testing.T) {
	rows := []map[string]interface{}{{"size": "7"}, {"size": "11"}}
	if got := backupTableBytes(rows, "size"); got != 18 {
		t.Fatalf("backupTableBytes() = %d, want 18", got)
	}
	if got := backupTableBytes([]map[string]interface{}{{"size": "-1"}}, "size"); got < 1<<60 {
		t.Fatalf("invalid size returned %d, want a quota-exceeding value", got)
	}
}
