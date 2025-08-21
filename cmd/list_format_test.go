package cmd

import (
  "bytes"
  "io"
  "os"
  "strings"
  "testing"
)

func TestPrintLockfileYAML(t *testing.T) {
  roots := []string{"a@1.0.0", "b@2.0.0"}
  pkgs := map[string]LockPackage{
    "a@1.0.0": {Name:"a", Version:"1.0.0", Dependencies: map[string]string{"b":"2.0.0"}},
    "b@2.0.0": {Name:"b", Version:"2.0.0"},
  }
  old := os.Stdout
  r, w, _ := os.Pipe()
  os.Stdout = w
  if err := printLockfileYAML(roots, pkgs); err != nil { t.Fatalf("yaml: %v", err) }
  w.Close(); os.Stdout = old
  b, _ := io.ReadAll(r); r.Close()
  s := string(b)
  if !strings.Contains(s, "roots:") || !strings.Contains(s, "packages:") { t.Fatalf("unexpected yaml: %s", s) }
}

