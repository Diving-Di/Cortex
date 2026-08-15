package migrations

import "testing"

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
