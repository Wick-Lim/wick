package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
  "testing"
)

func TestListLockfileRoots(t *testing.T) {
  proj := t.TempDir()
  lf := LockFile{Roots: []string{"a@1.0.0","b@2.0.0"}, Packages: map[string]LockPackage{
    "a@1.0.0": {Name:"a", Version:"1.0.0", Dependencies: map[string]string{}},
    "b@2.0.0": {Name:"b", Version:"2.0.0", Dependencies: map[string]string{}},
  }}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wlim.lock"), b, 0o644)

  roots, pkgs, err := listLockfile(proj)
  if err != nil { t.Fatalf("listLockfile: %v", err) }
  if len(roots) != 2 || len(pkgs) != 2 { t.Fatalf("unexpected sizes: roots=%d pkgs=%d", len(roots), len(pkgs)) }
}
