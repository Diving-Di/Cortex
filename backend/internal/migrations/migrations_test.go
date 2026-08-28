package migrations

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	values, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 {
		t.Fatal("no embedded migrations")
	}
	for index, value := range values {
		if index > 0 && values[index-1].Version >= value.Version {
			t.Fatal("migrations are not strictly ordered")
		}
		if value.UpSQL == "" || value.DownSQL == "" || len(value.Checksum) != 64 {
			t.Fatalf("invalid migration %#v", value)
		}
	}
}

func TestSchemaBaselineDoesNotPrecreatePendingMigrationTables(t *testing.T) {
	baseline, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(baseline), "Versions through 000013") {
		t.Fatal("schema baseline must declare its materialized migration version")
	}
	messagesStart := strings.Index(string(baseline), "CREATE TABLE public.messages (")
	if messagesStart < 0 {
		t.Fatal("messages table is missing from schema baseline")
	}
	messagesEnd := strings.Index(string(baseline)[messagesStart:], ");")
	if messagesEnd < 0 {
		t.Fatal("messages table definition is incomplete")
	}
	messagesDDL := string(baseline)[messagesStart : messagesStart+messagesEnd]
	for _, pendingColumn := range []string{"error_code character varying(64)", "upstream_stage character varying(64)", "output_tokens integer"} {
		if strings.Contains(messagesDDL, pendingColumn) {
			t.Fatalf("baseline precreates messages.%s owned by migration 000023", strings.Fields(pendingColumn)[0])
		}
	}
	createTable := regexp.MustCompile(`(?i)CREATE TABLE(?: IF NOT EXISTS)? public\.([a-z0-9_]+)`)
	baselineTables := map[string]bool{}
	for _, match := range createTable.FindAllStringSubmatch(string(baseline), -1) {
		baselineTables[strings.ToLower(match[1])] = true
	}
	values, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range values {
		if migration.Version <= 13 {
			continue
		}
		for _, match := range createTable.FindAllStringSubmatch(migration.UpSQL, -1) {
			if baselineTables[strings.ToLower(match[1])] {
				t.Fatalf("baseline precreates table %s owned by pending migration %06d", match[1], migration.Version)
			}
		}
	}
}

func TestAppliedMigrationChecksumsRemainStable(t *testing.T) {
	values, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int64]string{
		4: "643d5ca9a92b3bd9d01ac16955ce951d22108747ecf481fb9b7e02896c2bbd45",
	}
	for _, value := range values {
		if checksum, ok := expected[value.Version]; ok && value.Checksum != checksum {
			t.Fatalf("migration %d checksum changed: got %s want %s", value.Version, value.Checksum, checksum)
		}
		delete(expected, value.Version)
	}
	if len(expected) != 0 {
		t.Fatalf("expected migrations are missing: %#v", expected)
	}
}

func TestExecutableSQLMapsLegacyDatabaseRoles(t *testing.T) {
	input := "GRANT SELECT ON notes TO diary_app; ALTER DEFAULT PRIVILEGES FOR ROLE diary_migrator;"
	want := "GRANT SELECT ON notes TO cortex_app; ALTER DEFAULT PRIVILEGES FOR ROLE cortex_migrator;"
	if got := executableSQL(input); got != want {
		t.Fatalf("executableSQL() = %q, want %q", got, want)
	}
}

func TestChecksumAcceptedOnlyAllowsKnownLegacyValues(t *testing.T) {
	if !checksumAccepted(10, "e0fa4cb4fe22b92551fe9666520742e681870d79f76a1fffc776b1dddcb8936e", "current") {
		t.Fatal("known migration 10 checksum was rejected")
	}
	if checksumAccepted(10, "unknown", "current") {
		t.Fatal("unknown checksum was accepted")
	}
	if !checksumAccepted(99, "same", "same") {
		t.Fatal("matching checksum was rejected")
	}
}
