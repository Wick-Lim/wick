package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
  "testing"
)

func TestValidateProjectDetectsMissing(t *testing.T) {
  proj := t.TempDir()
  // Write lockfile with root a@1.0.0
  lf := LockFile{Roots: []string{"a@1.0.0"}, Packages: map[string]LockPackage{"a@1.0.0": {Name:"a", Version:"1.0.0"}}}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wick.lock"), b, 0o644)
  // No links or store -> validate should fail
  if err := validateProject(proj); err == nil {
    t.Fatalf("expected validate failure")
  }
}

