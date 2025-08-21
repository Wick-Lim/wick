package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
  "runtime"
  "testing"
)

func TestCleanRemovesUnreferenced(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  proj := t.TempDir()
  store := t.TempDir()
  // Create referenced x@1.0.0 and unreferenced y@1.0.0
  if err := os.MkdirAll(filepath.Join(store, "x", "1.0.0"), 0o755); err != nil { t.Fatal(err) }
  if err := os.MkdirAll(filepath.Join(store, "y", "1.0.0"), 0o755); err != nil { t.Fatal(err) }
  // Lockfile references x@1.0.0
  lf := LockFile{Roots: []string{"x@1.0.0"}, Packages: map[string]LockPackage{
    "x@1.0.0": {Name:"x", Version:"1.0.0"},
  }}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wlim.lock"), b, 0o644)

  // Run clean
  if err := cleanStore(proj, store, false); err != nil { t.Fatalf("cleanStore: %v", err) }

  if _, err := os.Stat(filepath.Join(store, "x", "1.0.0")); err != nil { t.Fatalf("referenced removed: %v", err) }
  if _, err := os.Stat(filepath.Join(store, "y", "1.0.0")); !os.IsNotExist(err) {
    t.Fatalf("unreferenced not removed")
  }
}
