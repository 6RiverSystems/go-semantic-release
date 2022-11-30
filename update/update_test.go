package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterApply(t *testing.T) {
	nVer := "1.2.3"
	Register("package.json", func(newVersion string, file *os.File) error {
		if newVersion != nVer {
			t.Fatal("invalid version")
		}
		return nil
	})
	td := t.TempDir()
	tdf := filepath.Join(td, "package.json")
	if err := os.WriteFile(tdf, []byte(testFixturePackageJson), 0o644); err != nil {
		t.Fatalf("failed to write test fixture file: %v", err)
	}
	if err := Apply(tdf, nVer); err != nil {
		t.Fatalf("failed to update package.json: %v", err)
	}
}

func TestApply(t *testing.T) {
	if err := Apply("invalidFile", ""); err != ErrNoUpdater {
		t.Fatal(err)
	}
	if err := Apply("not/existing/package.json", ""); !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatal(err)
	}
}
