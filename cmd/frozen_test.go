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

// When frozen-lockfile is used and lockfile references a version missing in registry, installation must fail.
func TestFrozenLockfileMismatchFails(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  var srv *httptest.Server
  handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/x":
      // Only 1.1.0 exists; lockfile will pin 1.0.0
      url11 := srv.URL + "/tar/x-1.1.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.1.0"}, Versions: map[string]PackageMetadata{
        "1.1.0": {Name:"x", Version:"1.1.0", Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: url11}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/x-1.1.0.tgz":
      pkg := []byte(`{"name":"x","version":"1.1.0"}`)
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
  // lockfile pins x@1.0.0 which is missing
  lf := LockFile{Roots: []string{"x@1.0.0"}, Packages: map[string]LockPackage{
    "x@1.0.0": {Name:"x", Version:"1.0.0"},
  }}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wick.lock"), b, 0o644)

  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  _, _, err := nodesFromLockfile(ctx, proj, cache)
  if err == nil {
    t.Fatalf("expected error due to missing version in registry")
  }
}
