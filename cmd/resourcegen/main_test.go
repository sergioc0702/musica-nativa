package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVersionSource(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "version.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAppVersionReadsTheVersionTheUpdaterReports(t *testing.T) {
	path := writeVersionSource(t, "package main\n\n// Bump together with build/config.yml.\nvar appVersion = \"2.3.4\"\n")
	got, err := appVersion(path)
	if err != nil || got != "2.3.4" {
		t.Fatalf("appVersion = %q, %v; want 2.3.4", got, err)
	}
}

func TestAppVersionRejectsSourcesWithoutStableVersion(t *testing.T) {
	for name, source := range map[string]string{
		"missing":    "package main\n",
		"not stable": "package main\n\nvar appVersion = \"dev\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := appVersion(writeVersionSource(t, source)); err == nil {
				t.Fatalf("appVersion = %q; want an error", got)
			}
		})
	}
}
