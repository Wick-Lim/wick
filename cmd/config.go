package cmd

import (
  "encoding/json"
  "os"
  "path/filepath"
)

type Config struct {
  Registry    string `json:"registry"`
  StoreDir    string `json:"storeDir"`
  Concurrency int    `json:"concurrency"`
}

func loadConfig(projectDir string) (*Config, error) {
  path := filepath.Join(projectDir, "wick.json")
  b, err := os.ReadFile(path)
  if err != nil { return &Config{}, nil }
  var cfg Config
  if err := json.Unmarshal(b, &cfg); err != nil { return &Config{}, nil }
  return &cfg, nil
}

func initProjectConfig(projectDir string, cfg *Config) error {
  path := filepath.Join(projectDir, "wick.json")
  if _, err := os.Stat(path); err == nil {
    // do not overwrite existing
    return nil
  }
  if cfg == nil { cfg = &Config{Concurrency: 0} }
  if err := os.MkdirAll(projectDir, 0o755); err != nil { return err }
  b, err := json.MarshalIndent(cfg, "", "  ")
  if err != nil { return err }
  return os.WriteFile(path, b, 0o644)
}
