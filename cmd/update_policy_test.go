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

func TestUpdatePolicyMinorPatch(t *testing.T) {
  var srv *httptest.Server
  handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/x":
      // Versions: 1.1.0(cur), 1.1.1(patch), 1.2.0(minor), 2.0.0(major)
      mk := func(v string) PackageMetadata { return PackageMetadata{Name:"x", Version:v, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: srv.URL+"/tar/x-"+v+".tgz"}} }
      doc := RootDoc{DistTags: map[string]string{"latest":"2.0.0"}, Versions: map[string]PackageMetadata{
        "1.1.0": mk("1.1.0"),
        "1.1.1": mk("1.1.1"),
        "1.2.0": mk("1.2.0"),
        "2.0.0": mk("2.0.0"),
      }}
      _ = json.NewEncoder(w).Encode(doc)
    default:
      // tar endpoints (not needed for this test)
      w.WriteHeader(200)
    }
  })
  srv = httptest.NewServer(handler)
  defer srv.Close()

  proj := t.TempDir()
  t.Setenv("WICK_CACHE_DIR", t.TempDir())
  t.Setenv("WICK_REGISTRY", srv.URL)
  // Starting at 1.1.0
  lf := LockFile{Roots: []string{"x@1.1.0"}, Packages: map[string]LockPackage{ "x@1.1.0": {Name:"x", Version:"1.1.0"}}}
  b,_ := json.Marshal(lf)
  _ = os.WriteFile(filepath.Join(proj, "wick.lock"), b, 0o644)

  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)

  // patch policy -> 1.1.1
  if err := updateLockfile(ctx, proj, []string{"x"}, cache, "patch", nil); err != nil { t.Fatalf("update patch: %v", err) }
  data, _ := os.ReadFile(filepath.Join(proj, "wick.lock"))
  var lfPatch LockFile
  _ = json.Unmarshal(data, &lfPatch)
  if lfPatch.Roots[0] != "x@1.1.1" { t.Fatalf("expected x@1.1.1, got %v", lfPatch.Roots[0]) }

  // minor policy -> 1.2.0
  if err := updateLockfile(ctx, proj, []string{"x"}, cache, "minor", nil); err != nil { t.Fatalf("update minor: %v", err) }
  data, _ = os.ReadFile(filepath.Join(proj, "wick.lock"))
  var lfMinor LockFile
  _ = json.Unmarshal(data, &lfMinor)
  if lfMinor.Roots[0] != "x@1.2.0" { t.Fatalf("expected x@1.2.0, got %v", lfMinor.Roots[0]) }
}
