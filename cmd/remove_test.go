package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
  "runtime"
  "testing"
)

func TestRemoveCommandBasics(t *testing.T) {
  if runtime.GOOS == "windows" {
    t.Skip("symlink behavior differs on Windows")
  }
  // This test assumes previous install tests or a pre-installed layout is present.
  // We create a fake project node_modules with symlinks and assert removal.
  project := t.TempDir()
  nm := filepath.Join(project, "node_modules")
  _ = os.MkdirAll(filepath.Join(nm, ".bin"), 0o755)

  // Create fake links
  store := t.TempDir()
  aDir := filepath.Join(store, "a", "1.0.0")
  _ = os.MkdirAll(aDir, 0o755)
  _ = os.Symlink(aDir, filepath.Join(nm, "a"))
  _ = os.Symlink(filepath.Join(aDir, "bin", "a.js"), filepath.Join(nm, ".bin", "a"))

  // Run removal
  if err := removeFromProject(project, []string{"a"}); err != nil {
    t.Fatalf("remove: %v", err)
  }
  if _, err := os.Lstat(filepath.Join(nm, "a")); !os.IsNotExist(err) {
    t.Fatalf("package link not removed")
  }
  if _, err := os.Lstat(filepath.Join(nm, ".bin", "a")); !os.IsNotExist(err) {
    t.Fatalf("bin link not removed")
  }
}

func TestRemoveUpdatesLockfile(t *testing.T) {
  proj := t.TempDir()
  lf := LockFile{Roots: []string{"a@1.0.0","b@1.0.0"}, Packages: map[string]LockPackage{
    "a@1.0.0": {Name:"a", Version:"1.0.0"},
    "b@1.0.0": {Name:"b", Version:"1.0.0"},
  }}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wlim.lock"), b, 0o644)

  if err := removeFromLockfile(proj, []string{"a"}); err != nil { t.Fatalf("removeFromLockfile: %v", err) }
  data, _ := os.ReadFile(filepath.Join(proj, "wlim.lock"))
  var lf2 LockFile
  _ = json.Unmarshal(data, &lf2)
  if len(lf2.Roots) != 1 || lf2.Roots[0] != "b@1.0.0" { t.Fatalf("unexpected roots: %+v", lf2.Roots) }
  if _, ok := lf2.Packages["a@1.0.0"]; ok { t.Fatalf("package a still present") }
}
