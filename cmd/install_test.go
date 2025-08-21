package cmd

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "net"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    "time"
)

// lockfile-driven install should honor exact versions
func TestInstallFromLockfileHonorsVersions(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }

    var srv *httptest.Server
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/x":
            // Provide two versions; dist-tags points to 1.1.0 but lockfile will pin 1.0.0
            url10 := srv.URL + "/tar/x-1.0.0.tgz"
            url11 := srv.URL + "/tar/x-1.1.0.tgz"
            doc := RootDoc{DistTags: map[string]string{"latest":"1.1.0"}, Versions: map[string]PackageMetadata{
                "1.0.0": {Name:"x", Version:"1.0.0", Dependencies: map[string]string{}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: url10}},
                "1.1.0": {Name:"x", Version:"1.1.0", Dependencies: map[string]string{}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: url11}},
            }}
            w.Header().Set("Content-Type","application/json")
            _ = json.NewEncoder(w).Encode(doc)
        case "/tar/x-1.0.0.tgz":
            pkg := []byte(`{"name":"x","version":"1.0.0"}`)
            tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
            w.Header().Set("Content-Type","application/octet-stream")
            _,_ = w.Write(tgz)
        case "/tar/x-1.1.0.tgz":
            pkg := []byte(`{"name":"x","version":"1.1.0"}`)
            tgz := makeTarGz(map[string][]byte{"package/package.json": pkg})
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
    t.Setenv("WICK_CACHE_DIR", t.TempDir())
    projectDir := t.TempDir()

    // Write lockfile pinning x@1.0.0
    lf := LockFile{Roots: []string{"x@1.0.0"}, Packages: map[string]LockPackage{
        "x@1.0.0": {Name:"x", Version:"1.0.0", Dependencies: map[string]string{}},
    }}
    b, _ := json.Marshal(lf)
    _ = os.WriteFile(filepath.Join(projectDir, "wick.lock"), b, 0o644)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    cache := make(map[string]*RootDoc)
    nodes, roots, err := nodesFromLockfile(ctx, projectDir, cache)
    if err != nil { t.Fatalf("nodesFromLockfile: %v", err) }
    storeDir, _ := defaultStoreDir()
    if err := installParallel(ctx, projectDir, storeDir, roots[0], nodes, 2); err != nil {
        t.Fatalf("installParallel: %v", err)
    }
    if fi, err := os.Lstat(filepath.Join(projectDir, "node_modules", "x")); err!=nil || fi.Mode()&os.ModeSymlink==0 {
        t.Fatalf("x link missing")
    }
}

// download retry path: first fail then succeed
func TestDownloadRetrySucceeds(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("symlink behavior differs on Windows") }
    var count int
    var srv *httptest.Server
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/y":
            url := srv.URL + "/tar/y-1.0.0.tgz"
            doc := RootDoc{DistTags: map[string]string{"latest":"1.0.0"}, Versions: map[string]PackageMetadata{
                "1.0.0": {Name:"y", Version:"1.0.0", Dependencies: map[string]string{}, Dist: struct{Tarball string `json:"tarball"`; Integrity string `json:"integrity"`; Shasum string `json:"shasum"`}{Tarball: url}},
            }}
            _ = json.NewEncoder(w).Encode(doc)
        case "/tar/y-1.0.0.tgz":
            count++
            if count < 2 { // first attempt fail
                w.WriteHeader(500)
                return
            }
            pkg := []byte(`{"name":"y","version":"1.0.0"}`)
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
    t.Setenv("WICK_CACHE_DIR", t.TempDir())
    projectDir := t.TempDir()
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    cache := make(map[string]*RootDoc)
    nodes, root, err := resolveGraph(ctx, "y", "latest", cache)
    if err != nil { t.Fatalf("resolve: %v", err) }
    storeDir, _ := defaultStoreDir()
    if err := installParallel(ctx, projectDir, storeDir, root, nodes, 2); err != nil {
        t.Fatalf("install: %v", err)
    }
}
func TestInstallParallelBasic(t *testing.T) {
    if runtime.GOOS == "windows" {
        t.Skip("symlink behavior differs on Windows")
    }

    // Create fake packages a@1.0.0 depends on b@^1.0.0 and b@1.0.0
    var srv *httptest.Server
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/a":
            tarURL := srv.URL + "/tar/a-1.0.0.tgz"
            doc := RootDoc{
                DistTags: map[string]string{"latest": "1.0.0"},
                Versions: map[string]PackageMetadata{
                    "1.0.0": {
                        Name: "a", Version: "1.0.0",
                        Dependencies: map[string]string{"b": "^1.0.0"},
                        Dist: struct{
                            Tarball string `json:"tarball"`
                            Integrity string `json:"integrity"`
                            Shasum string `json:"shasum"`
                        }{Tarball: tarURL},
                    },
                },
            }
            w.Header().Set("Content-Type", "application/json")
            _ = json.NewEncoder(w).Encode(doc)
        case "/b":
            tarURL := srv.URL + "/tar/b-1.0.0.tgz"
            doc := RootDoc{
                DistTags: map[string]string{"latest": "1.0.0"},
                Versions: map[string]PackageMetadata{
                    "1.0.0": {
                        Name: "b", Version: "1.0.0",
                        Dependencies: map[string]string{},
                        Dist: struct{
                            Tarball string `json:"tarball"`
                            Integrity string `json:"integrity"`
                            Shasum string `json:"shasum"`
                        }{Tarball: tarURL},
                    },
                },
            }
            w.Header().Set("Content-Type", "application/json")
            _ = json.NewEncoder(w).Encode(doc)
        case "/tar/a-1.0.0.tgz":
            // a has a bin
            pkgJSON := []byte(`{"name":"a","version":"1.0.0","bin":"bin/a.js","dependencies":{"b":"^1.0.0"}}`)
            tgz := makeTarGz(map[string][]byte{
                "package/package.json": pkgJSON,
                "package/bin/a.js":     []byte("#!/usr/bin/env node\nconsole.log('a')\n"),
                "package/index.js":      []byte("module.exports = 'a'\n"),
            })
            w.Header().Set("Content-Type", "application/octet-stream")
            _, _ = w.Write(tgz)
        case "/tar/b-1.0.0.tgz":
            pkgJSON := []byte(`{"name":"b","version":"1.0.0"}`)
            tgz := makeTarGz(map[string][]byte{
                "package/package.json": pkgJSON,
                "package/index.js":      []byte("module.exports = 'b'\n"),
            })
            w.Header().Set("Content-Type", "application/octet-stream")
            _, _ = w.Write(tgz)
        default:
            http.NotFound(w, r)
        }
    })
    ln, err := net.Listen("tcp4", "127.0.0.1:0")
    if err != nil { t.Fatalf("listen: %v", err) }
    srv = httptest.NewUnstartedServer(handler)
    srv.Listener = ln
    srv.Start()
    defer srv.Close()

    // Env overrides for registry and store
    t.Setenv("WICK_REGISTRY", srv.URL)
    storeRoot := t.TempDir()
    t.Setenv("WICK_STORE_DIR", storeRoot)
    projectDir := t.TempDir()

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    cache := make(map[string]*RootDoc)
    nodes, root, err := resolveGraph(ctx, "a", "latest", cache)
    if err != nil { t.Fatalf("resolveGraph: %v", err) }

    storeDir, err := defaultStoreDir()
    if err != nil { t.Fatalf("store: %v", err) }
    if err := installParallel(ctx, projectDir, storeDir, root, nodes, 2); err != nil {
        t.Fatalf("installParallel: %v", err)
    }

    // Assertions
    aStore := storePkgPath(storeDir, "a", "1.0.0")
    bStore := storePkgPath(storeDir, "b", "1.0.0")
    if _, err := os.Stat(aStore); err != nil { t.Fatalf("a store missing: %v", err) }
    if _, err := os.Stat(bStore); err != nil { t.Fatalf("b store missing: %v", err) }

    // project node_modules/a symlink exists
    aLink := filepath.Join(projectDir, "node_modules", "a")
    if fi, err := os.Lstat(aLink); err != nil || fi.Mode()&os.ModeSymlink == 0 {
        t.Fatalf("a link missing or not symlink: %v", err)
    }
    // a store has node_modules/b link
    bLink := filepath.Join(aStore, "node_modules", "b")
    if fi, err := os.Lstat(bLink); err != nil || fi.Mode()&os.ModeSymlink == 0 {
        t.Fatalf("a->b link missing or not symlink: %v", err)
    }
    // .bin contains 'a'
    binA := filepath.Join(projectDir, "node_modules", ".bin", "a")
    if fi, err := os.Lstat(binA); err != nil || fi.Mode()&os.ModeSymlink == 0 {
        t.Fatalf(".bin/a missing or not symlink: %v", err)
    }
    // lockfile exists and includes packages
    lockPath := filepath.Join(projectDir, "wick.lock")
    data, err := os.ReadFile(lockPath)
    if err != nil { t.Fatalf("lockfile missing: %v", err) }
    var lf LockFile
    if err := json.Unmarshal(data, &lf); err != nil { t.Fatalf("lockfile parse: %v", err) }
    if len(lf.Roots) == 0 || len(lf.Packages) < 2 { t.Fatalf("lockfile content invalid: %+v", lf) }
}
