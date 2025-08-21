package cmd

import (
  "context"
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "testing"
  "time"
)

// Ensure --registry (registryOverride) takes precedence over WICK_REGISTRY
func TestRegistryFlagPrecedence(t *testing.T) {
  bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    w.WriteHeader(500)
  }))
  defer bad.Close()
  good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    if r.URL.Path == "/pkg" {
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"pkg", Version:"1.0.0"},
      }}
      _ = json.NewEncoder(w).Encode(doc)
      return
    }
    w.WriteHeader(404)
  }))
  defer good.Close()

  t.Setenv("WICK_REGISTRY", bad.URL)
  registryOverride = good.URL
  defer func(){ registryOverride = "" }()

  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  rd, err := fetchRootDoc(ctx, "pkg", cache)
  if err != nil { t.Fatalf("fetchRootDoc failed: %v", err) }
  if rd.DistTags["latest"] != "1.0.0" { t.Fatalf("unexpected doc: %+v", rd) }
}

