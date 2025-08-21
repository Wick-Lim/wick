package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
  "testing"
)

func TestInitCreatesConfig(t *testing.T) {
  proj := t.TempDir()
  if err := initProjectConfig(proj, &Config{Registry:"http://r", StoreDir:"/s", Concurrency:4}); err != nil {
    t.Fatalf("init: %v", err)
  }
  b, err := os.ReadFile(filepath.Join(proj, "wlim.json"))
  if err != nil { t.Fatalf("read: %v", err) }
  var cfg Config
  if err := json.Unmarshal(b, &cfg); err != nil { t.Fatalf("json: %v", err) }
  if cfg.Registry != "http://r" || cfg.StoreDir != "/s" || cfg.Concurrency != 4 {
    t.Fatalf("unexpected cfg: %+v", cfg)
  }
}
