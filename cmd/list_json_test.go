package cmd

import (
  "encoding/json"
  "io"
  "os"
  "testing"
)

func TestPrintLockfileJSON(t *testing.T) {
  roots := []string{"a@1.0.0"}
  pkgs := map[string]LockPackage{"a@1.0.0": {Name:"a", Version:"1.0.0"}}
  // temporarily swap stdout
  old := os.Stdout
  r, w, _ := os.Pipe()
  os.Stdout = w
  if err := printLockfileJSON(roots, pkgs); err != nil { t.Fatalf("print: %v", err) }
  w.Close()
  os.Stdout = old
  // read
  b, _ := io.ReadAll(r)
  r.Close()
  var out struct{ Roots []string; Packages map[string]LockPackage }
  if err := json.Unmarshal(b, &out); err != nil { t.Fatalf("json: %v", err) }
  if len(out.Roots) != 1 || len(out.Packages) != 1 { t.Fatalf("unexpected: %+v", out) }
}
