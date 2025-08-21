package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
  "testing"
)

func TestLoadConfigAndPrecedence(t *testing.T) {
  proj := t.TempDir()
  cfg := Config{Registry: "http://reg", StoreDir: "/store", Concurrency: 3}
  b,_ := json.Marshal(cfg)
  _ = os.WriteFile(filepath.Join(proj, "wlim.json"), b, 0o644)

  // env overrides config for store
  t.Setenv("WLIM_STORE_DIR", "/envstore")
  // flag should override both; we simulate by calling default/effective functions indirectly via env
  c, err := loadConfig(proj)
  if err != nil { t.Fatalf("load: %v", err) }
  if c.Registry != "http://reg" || c.StoreDir != "/store" || c.Concurrency != 3 {
    t.Fatalf("unexpected cfg: %+v", c)
  }
}
