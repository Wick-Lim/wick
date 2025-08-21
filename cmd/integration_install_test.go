package cmd

import (
  "context"
  "os"
  "path/filepath"
  "runtime"
  "testing"
  "time"
)

func TestInstallEscapeStringRegexp(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  t.Setenv("WLIM_CACHE_DIR", t.TempDir())
  t.Setenv("WLIM_STORE_DIR", t.TempDir())
  projectDir := t.TempDir()

  ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  pkg := "escape-string-regexp"
  nodes, root, err := resolveGraph(ctx, pkg, "latest", cache)
  if err != nil { t.Fatalf("resolveGraph: %v", err) }
  storeDir, err := defaultStoreDir()
  if err != nil { t.Fatalf("store: %v", err) }
  if err := installParallel(ctx, projectDir, storeDir, root, nodes, 2); err != nil {
    t.Fatalf("installParallel: %v", err)
  }
  if _, err := os.Lstat(filepath.Join(projectDir, "node_modules", pkg)); err != nil {
    t.Fatalf("project link missing: %v", err)
  }
}

func TestInstallTwoPackagesIntegration(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  t.Setenv("WLIM_CACHE_DIR", t.TempDir())
  t.Setenv("WLIM_STORE_DIR", t.TempDir())
  projectDir := t.TempDir()

  ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  pkgs := []string{"escape-string-regexp", "is-number"}
  allNodes := map[string]*GraphNode{}
  var roots []*GraphNode
  for _, p := range pkgs {
    nodes, root, err := resolveGraph(ctx, p, "latest", cache)
    if err != nil { t.Fatalf("resolveGraph(%s): %v", p, err) }
    for k, n := range nodes { allNodes[k] = n }
    roots = append(roots, root)
  }
  storeDir, err := defaultStoreDir()
  if err != nil { t.Fatalf("store: %v", err) }
  if err := installGraph(ctx, projectDir, storeDir, roots, allNodes, 3); err != nil {
    t.Fatalf("installGraph: %v", err)
  }
  for _, p := range pkgs {
    if _, err := os.Lstat(filepath.Join(projectDir, "node_modules", p)); err != nil {
      t.Fatalf("project link missing for %s: %v", p, err)
    }
  }
}
