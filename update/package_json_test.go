package update

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testFixturePackageJson string = `{
  "name": "test-package",
  "version": "1.0.0"
}`

func TestPackageJson(t *testing.T) {
	// don't modify the input file, make a copy and check that
	td := t.TempDir()
	tdf := filepath.Join(td, "package.json")
	f, err := os.OpenFile(tdf, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0)
	if err != nil {
		t.Fatalf("unable to open test file %s: %v", tdf, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, strings.NewReader(testFixturePackageJson)); err != nil {
		t.Fatalf("unable to write test file: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unable to rewind test file: %v", err)
	}
	newVersion := "1.2.3"
	if err := packageJson(newVersion, f); err != nil {
		t.Fatalf("failed package json update: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("failed to rewind test file for verification: %v", err)
	}
	var data map[string]json.RawMessage
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		t.Fatalf("failed to parse updated package.json: %v", err)
	}
	if !bytes.Equal(data["version"], []byte("\""+newVersion+"\"")) {
		t.Fatalf("invalid version: expect \"%s\", got %s", newVersion, string(data["version"]))
	}
}
