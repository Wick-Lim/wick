package cmd

import (
  "context"
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "os"
  "path/filepath"
  "testing"
  "time"
)

// Passing name@version to update should pin to that exact version ignoring policy
func TestUpdateWithExplicitSpec(t *testing.T) {
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
    default:
      w.WriteHeader(200)
    }
  })
  srv = httptest.NewServer(handler)
  defer srv.Close()
  os.Setenv("WICK_REGISTRY", srv.URL)

  proj := t.TempDir()
  lf := LockFile{Roots: []string{"x@1.0.0"}, Packages: map[string]LockPackage{ "x@1.0.0": {Name:"x", Version:"1.0.0"} }}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wick.lock"), b, 0o644)

  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  // Ask to update to 1.0.0 explicitly while policy=latest would prefer 1.1.0
  if err := updateLockfile(ctx, proj, []string{"x"}, cache, "latest", map[string]string{"x":"1.0.0"}); err != nil {
    t.Fatalf("update: %v", err)
  }
  data, _ := os.ReadFile(filepath.Join(proj, "wick.lock"))
  var lf2 LockFile
  _ = json.Unmarshal(data, &lf2)
  if lf2.Roots[0] != "x@1.0.0" { t.Fatalf("expected x@1.0.0, got %v", lf2.Roots[0]) }
}

