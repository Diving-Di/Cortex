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
