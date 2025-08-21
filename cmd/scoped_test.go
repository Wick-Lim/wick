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

func TestScopedPackageInstall(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  var srv *httptest.Server
  handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/@scope/name":
      url := srv.URL + "/tar/scope-name-1.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"@scope/name", Version:"1.0.0", Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: url}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/scope-name-1.0.0.tgz":
      pkg := []byte(`{"name":"@scope/name","version":"1.0.0"}`)
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
  proj := t.TempDir()
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  nodes, root, err := resolveGraph(ctx, "@scope/name", "latest", cache)
  if err != nil { t.Fatalf("resolve: %v", err) }
  store, _ := defaultStoreDir()
  if err := installParallel(ctx, proj, store, root, nodes, 2); err != nil { t.Fatalf("install: %v", err) }
  if fi, err := os.Lstat(filepath.Join(proj, "node_modules", "@scope", "name")); err!=nil || fi.Mode()&os.ModeSymlink==0 {
    t.Fatalf("scoped link missing")
  }
}

