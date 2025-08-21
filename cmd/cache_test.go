package cmd

import (
  "context"
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "testing"
  "time"
)

// Ensure disk cache is used when registry unavailable
func TestRegistryDiskCache(t *testing.T) {
  // 1st server serves metadata
  srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    if r.URL.Path == "/pkg" {
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"pkg", Version:"1.0.0"},
      }}
      _ = json.NewEncoder(w).Encode(doc)
      return
    }
    w.WriteHeader(404)
  }))
  // Set cache dir
  t.Setenv("WICK_CACHE_DIR", t.TempDir())
  t.Setenv("WICK_REGISTRY", srv.URL)
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  // first fetch writes cache
  if _, err := fetchRootDoc(ctx, "pkg", nil); err != nil { t.Fatalf("first fetch: %v", err) }
  srv.Close()
  // second fetch should read from cache (no registry)
  if _, err := fetchRootDoc(ctx, "pkg", nil); err != nil { t.Fatalf("second fetch from cache: %v", err) }
}
