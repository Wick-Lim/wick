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

func TestUpdateLockfileBumpsRootVersion(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  var srv *httptest.Server
  handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/x":
      u10 := srv.URL + "/tar/x-1.0.0.tgz"
      u11 := srv.URL + "/tar/x-1.1.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.1.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"x", Version:"1.0.0", Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: u10}},
        "1.1.0": {Name:"x", Version:"1.1.0", Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: u11}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/x-1.0.0.tgz":
      pkg := []byte(`{"name":"x","version":"1.0.0"}`)
      tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
      _,_ = w.Write(tgz)
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

  proj := t.TempDir()
  t.Setenv("WICK_STORE_DIR", t.TempDir())
  // Initial lockfile is 1.0.0
  lf := LockFile{Roots: []string{"x@1.0.0"}, Packages: map[string]LockPackage{
    "x@1.0.0": {Name:"x", Version:"1.0.0"},
  }}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wick.lock"), b, 0o644)

  ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  // Update x to latest (1.1.0)
  if err := updateLockfile(ctx, proj, []string{"x"}, cache, "latest", nil); err != nil {
    t.Fatalf("updateLockfile: %v", err)
  }

  data, err := os.ReadFile(filepath.Join(proj, "wick.lock"))
  if err != nil { t.Fatalf("read lock: %v", err) }
  var lf2 LockFile
  if err := json.Unmarshal(data, &lf2); err != nil { t.Fatalf("parse lock: %v", err) }
  ok := false
  for _, r := range lf2.Roots { if r == "x@1.1.0" { ok = true } }
  if !ok { t.Fatalf("expected root x@1.1.0, got %+v", lf2.Roots) }
}
