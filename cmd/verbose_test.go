package cmd

import (
  "context"
  "encoding/json"
  "io"
  "net/http"
  "net/http/httptest"
  "os"
  "path/filepath"
  "runtime"
  "strings"
  "testing"
  "time"
)

func TestVerboseLogsDuringInstall(t *testing.T) {
  if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
  srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/a":
      url := srv.URL + "/tar/a-1.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"a", Version:"1.0.0", Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: url}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/a-1.0.0.tgz":
      pkg := []byte(`{"name":"a","version":"1.0.0"}`)
      tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
      _,_ = w.Write(tgz)
    default:
      w.WriteHeader(404)
    }
  }))
  defer srv.Close()
  t.Setenv("WICK_REGISTRY", srv.URL)
  t.Setenv("WICK_STORE_DIR", t.TempDir())
  proj := t.TempDir()

  // capture stdout
  old := os.Stdout
  r, w, _ := os.Pipe()
  os.Stdout = w
  verbose = true
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)
  nodes, root, err := resolveGraph(ctx, "a", "latest", cache)
  if err != nil { t.Fatalf("resolve: %v", err) }
  store, _ := defaultStoreDir()
  if err := installParallel(ctx, proj, store, root, nodes, 1); err != nil { t.Fatalf("install: %v", err) }
  verbose = false
  w.Close()
  os.Stdout = old
  b, _ := io.ReadAll(r)
  out := string(b)
  if !strings.Contains(out, "Downloading a@1.0.0") && !strings.Contains(out, "Installed a@1.0.0") {
    t.Fatalf("expected verbose logs, got: %s", out)
  }
}

