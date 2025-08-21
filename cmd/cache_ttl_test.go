package cmd

import (
  "context"
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "os"
  "testing"
  "time"
)

func TestCacheTTLExpiresAndRefetches(t *testing.T) {
  // Server returns latest 1.0.1
  srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    if r.URL.Path == "/pkg" {
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.1"}}
      _ = json.NewEncoder(w).Encode(doc)
      return
    }
    w.WriteHeader(404)
  }))
  defer srv.Close()

  // Seed cache with 1.0.0 and backdate mtime
  cacheDir := t.TempDir()
  t.Setenv("WICK_CACHE_DIR", cacheDir)
  _ = writeRootDocCache("pkg", &RootDoc{DistTags: map[string]string{"latest":"1.0.0"}})
  p, _ := rootDocCachePath("pkg")
  old := time.Now().Add(-2 * time.Hour)
  _ = os.Chtimes(p, old, old)

  t.Setenv("WICK_CACHE_TTL_SECONDS", "1")
  t.Setenv("WICK_REGISTRY", srv.URL)

  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  rd, err := fetchRootDoc(ctx, "pkg", nil)
  if err != nil { t.Fatalf("fetch: %v", err) }
  if rd.DistTags["latest"] != "1.0.1" { t.Fatalf("expected refetch to 1.0.1, got %v", rd.DistTags["latest"]) }
}
