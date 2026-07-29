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
