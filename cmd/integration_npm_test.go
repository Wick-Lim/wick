package cmd

import (
  "context"
  "os"
  "path/filepath"
  "runtime"
  "testing"
  "time"
)

// This test hits the real npm registry. It is skipped by default.
// Enable with: WICK_INTEGRATION=1 go test -v -run TestNPMIntegration ./cmd
func TestNPMIntegration(t *testing.T) {
  if os.Getenv("WICK_INTEGRATION") != "1" {
    t.Skip("set WICK_INTEGRATION=1 to run npm integration test")
  }
  if runtime.GOOS == "windows" {
    t.Skip("symlink behavior differs on Windows; integration skipped")
  }

  // Use default registry (registry.npmjs.org). Isolate caches/stores.
  t.Setenv("WICK_CACHE_DIR", t.TempDir())
  t.Setenv("WICK_STORE_DIR", t.TempDir())
  projectDir := t.TempDir()

  ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)

  // Pick a very small, dependency-free package
  const pkg = "escape-string-regexp" // commonly dependency-free and lightweight
  nodes, root, err := resolveGraph(ctx, pkg, "latest", cache)
  if err != nil { t.Fatalf("resolveGraph: %v", err) }

  storeDir, err := defaultStoreDir()
  if err != nil { t.Fatalf("store: %v", err) }
  if err := installParallel(ctx, projectDir, storeDir, root, nodes, 2); err != nil {
    t.Fatalf("install: %v", err)
  }

  // Validate link exists
  link := filepath.Join(projectDir, "node_modules", pkg)
  if _, err := os.Lstat(link); err != nil {
    t.Fatalf("project link missing: %v", err)
  }
}

