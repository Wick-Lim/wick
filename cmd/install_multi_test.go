package cmd

import (
  "context"
  "encoding/json"
  "errors"
  "net"
  "net/http"
  "net/http/httptest"
  "os"
  "path/filepath"
  "runtime"
  "testing"
  "time"
)

func TestInstallMultipleRoots(t *testing.T) {
  if runtime.GOOS == "windows" {
    t.Skip("symlink behavior differs on Windows")
  }

  var srv *httptest.Server
  handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/a":
      tarURL := srv.URL + "/tar/a-1.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"a", Version:"1.0.0", Dependencies: map[string]string{}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: tarURL}},
      }}
      w.Header().Set("Content-Type","application/json")
      _ = json.NewEncoder(w).Encode(doc)
    case "/c":
      tarURL := srv.URL + "/tar/c-2.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"2.0.0"}, Versions: map[string]PackageMetadata{
        "2.0.0": {Name:"c", Version:"2.0.0", Dependencies: map[string]string{}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: tarURL}},
      }}
      w.Header().Set("Content-Type","application/json")
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/a-1.0.0.tgz":
      pkg := []byte(`{"name":"a","version":"1.0.0","bin":"bin/a.js"}`)
      tgz := makeTarGz(map[string][]byte{
        "package/package.json": pkg,
        "package/bin/a.js": []byte("#!/usr/bin/env node\nconsole.log('a')\n"),
      })
      w.Header().Set("Content-Type","application/octet-stream")
      _,_ = w.Write(tgz)
    case "/tar/c-2.0.0.tgz":
      pkg := []byte(`{"name":"c","version":"2.0.0","bin":"bin/c.js"}`)
      tgz := makeTarGz(map[string][]byte{
        "package/package.json": pkg,
        "package/bin/c.js": []byte("#!/usr/bin/env node\nconsole.log('c')\n"),
      })
      w.Header().Set("Content-Type","application/octet-stream")
      _,_ = w.Write(tgz)
    default:
      http.NotFound(w,r)
    }
  })
  ln, err := net.Listen("tcp4","127.0.0.1:0")
  if err != nil { t.Fatalf("listen: %v", err) }
  srv = httptest.NewUnstartedServer(handler)
  srv.Listener = ln
  srv.Start()
  defer srv.Close()

  t.Setenv("WICK_REGISTRY", srv.URL)
  t.Setenv("WICK_STORE_DIR", t.TempDir())
  projectDir := t.TempDir()

  ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  defer cancel()
  cache := make(map[string]*RootDoc)

  // Resolve graphs individually and merge by calling twice in install (command supports multi args)
  nodesA, rootA, err := resolveGraph(ctx, "a", "latest", cache)
  if err != nil { t.Fatalf("resolve a: %v", err) }
  nodesC, rootC, err := resolveGraph(ctx, "c", "latest", cache)
  if err != nil { t.Fatalf("resolve c: %v", err) }
  // Merge maps
  nodes := map[string]*GraphNode{}
  for k,v := range nodesA { nodes[k]=v }
  for k,v := range nodesC { nodes[k]=v }

  storeDir, _ := defaultStoreDir()
  if err := installParallel(ctx, projectDir, storeDir, rootA, nodes, 4); err != nil {
    t.Fatalf("install a: %v", err)
  }
  // Link second root manually (simulate CLI behavior)
  if err := installParallel(ctx, projectDir, storeDir, rootC, nodes, 4); err != nil {
    t.Fatalf("install c: %v", err)
  }

  // Expect both installed and bins linked
  for _, name := range []string{"a","c"} {
    if fi, err := os.Lstat(filepath.Join(projectDir,"node_modules",name)); err!=nil || fi.Mode()&os.ModeSymlink==0 {
      t.Fatalf("%s link missing", name)
    }
    if fi, err := os.Lstat(filepath.Join(projectDir,"node_modules",".bin",name)); err!=nil || fi.Mode()&os.ModeSymlink==0 {
      t.Fatalf(".bin/%s link missing", name)
    }
  }

  // Integrity mismatch should error
  called := 0
  var bad *httptest.Server
  handler2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
    switch r.URL.Path {
    case "/z":
      tarURL := bad.URL + "/tar/z-1.0.0.tgz"
      doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
        "1.0.0": {Name:"z", Version:"1.0.0", Dependencies: map[string]string{}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: tarURL, Integrity: "sha512-AAAAAAAA"}},
      }}
      _ = json.NewEncoder(w).Encode(doc)
    case "/tar/z-1.0.0.tgz":
      called++
      pkg := []byte(`{"name":"z","version":"1.0.0"}`)
      tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
      _,_ = w.Write(tgz)
    default:
      http.NotFound(w,r)
    }
  })
  bad = httptest.NewServer(handler2)
  defer bad.Close()
  t.Setenv("WICK_REGISTRY", bad.URL)
  cache = make(map[string]*RootDoc)
  ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  n2, r2, err := resolveGraph(ctx, "z", "latest", cache)
  if err != nil { t.Fatal(err) }
  storeDir, _ = defaultStoreDir()
  err = installParallel(ctx, projectDir, storeDir, r2, n2, 2)
  if err == nil {
    t.Fatalf("expected integrity error, got nil")
  }
  if called == 0 { t.Fatal(errors.New("tar endpoint not called")) }
}
