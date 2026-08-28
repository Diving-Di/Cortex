package application_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Business HTTP handlers must enter through an application service. Process
// probes and metrics are deliberately excluded because they inspect runtime
// infrastructure rather than execute tenant use cases.
func TestBusinessHandlersDoNotCallStoreDirectly(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	serverDir := filepath.Join(filepath.Dir(current), "..", "server")
	entries, err := os.ReadDir(serverDir)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"server.go": true, "metrics.go": true}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_worker.go") || allowed[name] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(serverDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "s.store.") {
			t.Errorf("%s calls Store directly; route the use case through internal/application", name)
		}
	}
}

func TestApplicationDoesNotDependOnHTTPServer(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Dir(current)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		forbidden := `"cortex/backend/internal/` + `server"`
		if strings.Contains(string(data), forbidden) {
			t.Errorf("%s imports the HTTP layer", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
