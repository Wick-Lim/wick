package cmd

import (
  "context"
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "os"
  "path/filepath"
  "runtime"
  "testing"
  "time"
)

func TestCyclicDependenciesInstall(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  var srv *httptest.Server
  handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/a":
      ta := srv.URL + "/tar/a-1.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"a", Version:"1.0.0", Dependencies: map[string]string{"b":"1.0.0"}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: ta}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/b":
      tb := srv.URL + "/tar/b-1.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"b", Version:"1.0.0", Dependencies: map[string]string{"a":"1.0.0"}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: tb}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/a-1.0.0.tgz":
      pkg := []byte(`{"name":"a","version":"1.0.0"}`)
      tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
      _,_ = w.Write(tgz)
    case "/tar/b-1.0.0.tgz":
      pkg := []byte(`{"name":"b","version":"1.0.0"}`)
      tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
      _,_ = w.Write(tgz)
    default:
      http.NotFound(w,r)
    }
  })
  srv = httptest.NewServer(handler)
  defer srv.Close()
  t.Setenv("WICK_REGISTRY", srv.URL)
  t.Setenv("WICK_STORE_DIR", t.TempDir())
  projectDir := t.TempDir()

  ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  nodes, root, err := resolveGraph(ctx, "a", "latest", cache)
  if err != nil { t.Fatalf("resolve: %v", err) }
  storeDir, _ := defaultStoreDir()
  if err := installParallel(ctx, projectDir, storeDir, root, nodes, 2); err != nil { t.Fatalf("install: %v", err) }
  // Both a and b exist
  for _, name := range []string{"a","b"} {
    if fi, err := os.Lstat(filepath.Join(projectDir, "node_modules", name)); err!=nil || fi.Mode()&os.ModeSymlink==0 {
      t.Fatalf("%s link missing", name)
    }
  }
}
