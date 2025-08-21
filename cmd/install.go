package cmd

import (
    "archive/tar"
    "compress/gzip"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "crypto/sha1"
    "crypto/sha512"
    "encoding/base64"
    "encoding/hex"
    "net/http"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "strconv"
    "time"
    "runtime"

    semver "github.com/Masterminds/semver/v3"
    "github.com/spf13/cobra"
)

// PackageMetadata 구조체 정의
type PackageMetadata struct {
    Name         string            `json:"name"`
    Version      string            `json:"version"`
    Dependencies map[string]string `json:"dependencies"`
    Dist         struct {
        Tarball   string `json:"tarball"`
        Integrity string `json:"integrity"`
        Shasum    string `json:"shasum"`
    } `json:"dist"`
}

// 루트 문서(전체 버전들을 포함)
type RootDoc struct {
    DistTags map[string]string             `json:"dist-tags"`
    Versions map[string]PackageMetadata    `json:"versions"`
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// Visual logging helpers
var logFormat = "fancy" // fancy|plain
var logNoColor = false
var showProgress = true

func colorize(code, s string) string {
    if logNoColor { return s }
    return code + s + "\x1b[0m"
}

func vLogFancy(icon, msg, color string) {
    if logFormat != "fancy" { return }
    if color != "" { msg = colorize(color, msg) }
    fmt.Println(icon, msg)
}

func vLogPlain(prefix, msg string) {
    if logFormat != "plain" { return }
    if prefix != "" { fmt.Printf("[%s] %s\n", prefix, msg) } else { fmt.Println(msg) }
}

func vStage(stage, pkg, ver string) {
    switch stage {
    case "fetch":
        vLogFancy("⟲", fmt.Sprintf("Fetching %s@%s", pkg, ver), "\x1b[36m")
        vLogPlain("fetch", fmt.Sprintf("%s@%s", pkg, ver))
    case "extract":
        vLogFancy("⇣", fmt.Sprintf("Extracting %s@%s", pkg, ver), "\x1b[34m")
        vLogPlain("extract", fmt.Sprintf("%s@%s", pkg, ver))
    case "link-deps":
        vLogFancy("↳", fmt.Sprintf("Link deps for %s@%s", pkg, ver), "\x1b[33m")
        vLogPlain("link-deps", fmt.Sprintf("%s@%s", pkg, ver))
    case "link-root":
        vLogFancy("↗", fmt.Sprintf("Link %s@%s into project", pkg, ver), "\x1b[35m")
        vLogPlain("link-root", fmt.Sprintf("%s@%s", pkg, ver))
    case "done":
        vLogFancy("✓", fmt.Sprintf("Installed %s@%s", pkg, ver), "\x1b[32m")
        vLogPlain("done", fmt.Sprintf("%s@%s", pkg, ver))
    }
}

func getJSON(ctx context.Context, url string, target any) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return err
    }
    req.Header.Set("Accept", "application/json")
    resp, err := httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
    }
    dec := json.NewDecoder(resp.Body)
    return dec.Decode(target)
}

func registryBase() string {
    if registryOverride != "" {
        return strings.TrimRight(registryOverride, "/")
    }
    if v := os.Getenv("WLIM_REGISTRY"); v != "" {
        return strings.TrimRight(v, "/")
    }
    return "https://registry.npmjs.org"
}

var registryOverride string

// simple retry with backoff for HTTP GET
func getWithRetry(ctx context.Context, url string, maxAttempts int) (*http.Response, error) {
    var lastErr error
    backoff := 300 * time.Millisecond
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
        if err != nil {
            return nil, err
        }
        req.Header.Set("Accept", "application/octet-stream")
        resp, err := httpClient.Do(req)
        if err == nil && resp.StatusCode == http.StatusOK {
            return resp, nil
        }
        if resp != nil {
            // close body to avoid leaks
            io.Copy(io.Discard, resp.Body)
            resp.Body.Close()
            // retry only for 429 and 5xx
            if resp.StatusCode != http.StatusTooManyRequests && (resp.StatusCode < 500 || resp.StatusCode >= 600) {
                return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
            }
        }
        if err != nil {
            lastErr = err
        } else {
            lastErr = fmt.Errorf("GET %s failed", url)
        }
        select {
        case <-time.After(backoff):
            backoff *= 2
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    if lastErr == nil {
        lastErr = fmt.Errorf("failed to GET %s", url)
    }
    return nil, lastErr
}

// 패키지 루트 메타데이터 조회(캐시 지원)
func fetchRootDoc(ctx context.Context, packageName string, cache map[string]*RootDoc) (*RootDoc, error) {
    if cache != nil {
        if rd, ok := cache[packageName]; ok {
            return rd, nil
        }
    }
    // disk cache with TTL
    if useCache, rd := tryReadRootDocCacheWithTTL(packageName); useCache && rd != nil {
        if cache != nil { cache[packageName] = rd }
        return rd, nil
    }
    base := registryBase()
    url := fmt.Sprintf("%s/%s", base, packageName)
    var rd RootDoc
    if err := getJSON(ctx, url, &rd); err != nil {
        return nil, err
    }
    _ = writeRootDocCache(packageName, &rd)
    if cache != nil {
        cache[packageName] = &rd
    }
    return &rd, nil
}

// 버전 스펙을 실제 버전으로 해석하고 해당 메타데이터 반환
func resolveVersionAndMetadata(ctx context.Context, name, spec string, cache map[string]*RootDoc) (string, *PackageMetadata, error) {
    rd, err := fetchRootDoc(ctx, name, cache)
    if err != nil {
        return "", nil, err
    }

    // 1) latest 또는 빈값
    if spec == "" || spec == "latest" {
        v := rd.DistTags["latest"]
        if v == "" {
            return "", nil, fmt.Errorf("no latest dist-tag for %s", name)
        }
        md, ok := rd.Versions[v]
        if !ok {
            return "", nil, fmt.Errorf("version %s not found for %s", v, name)
        }
        return v, &md, nil
    }

    // 2) 정확한 버전 존재 시
    if md, ok := rd.Versions[spec]; ok {
        v := spec
        copy := md
        return v, &copy, nil
    }

    // 3) semver 제약 조건으로 해석
    cons, err := semver.NewConstraint(spec)
    if err != nil {
        return "", nil, fmt.Errorf("invalid version spec %q for %s: %w", spec, name, err)
    }

    // 가능한 버전 수집 후 정렬하여 최대 만족 버전 선택
    var versions semver.Collection
    versionMap := make(map[string]PackageMetadata, len(rd.Versions))
    for vStr, md := range rd.Versions {
        v, err := semver.NewVersion(vStr)
        if err != nil {
            continue
        }
        if cons.Check(v) {
            versions = append(versions, v)
            versionMap[v.Original()] = md
        }
    }
    if len(versions) == 0 {
        return "", nil, fmt.Errorf("no versions of %s satisfy %q", name, spec)
    }
    sort.Sort(versions)
    chosen := versions[len(versions)-1].Original()
    md := versionMap[chosen]
    return chosen, &md, nil
}

func ensureDir(path string) error {
    return os.MkdirAll(path, 0o755)
}

// 안전한 경로 결합(경로 탈출 방지)
func safeJoin(base, rel string) (string, error) {
    cleaned := filepath.Join(base, rel)
    if !strings.HasPrefix(cleaned, filepath.Clean(base)+string(os.PathSeparator)) && filepath.Clean(base) != cleaned {
        return "", fmt.Errorf("unsafe path: %s", rel)
    }
    return cleaned, nil
}

func downloadAndExtractFromFile(tarGzPath, destDir string) error {
    if err := ensureDir(destDir); err != nil {
        return err
    }
    f, err := os.Open(tarGzPath)
    if err != nil {
        return err
    }
    defer f.Close()
    gz, err := gzip.NewReader(f)
    if err != nil {
        return fmt.Errorf("gzip: %w", err)
    }
    defer gz.Close()
    tr := tar.NewReader(gz)

    for {
        hdr, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        name := hdr.Name
        // npm tarballs usually prefix with "package/"
        if parts := strings.SplitN(name, "/", 2); len(parts) == 2 {
            name = parts[1]
        } else {
            // If no slash, skip root folder entries
            continue
        }

        targetPath, err := safeJoin(destDir, name)
        if err != nil {
            return err
        }

        switch hdr.Typeflag {
        case tar.TypeDir:
            if err := ensureDir(targetPath); err != nil {
                return err
            }
        case tar.TypeReg, tar.TypeRegA:
            if err := ensureDir(filepath.Dir(targetPath)); err != nil {
                return err
            }
            f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
            if err != nil {
                return err
            }
            if _, err := io.Copy(f, tr); err != nil {
                f.Close()
                return err
            }
            f.Close()
        case tar.TypeSymlink:
            // Best-effort: create symlink if possible; ignore failures
            if err := ensureDir(filepath.Dir(targetPath)); err != nil {
                return err
            }
            _ = os.Symlink(hdr.Linkname, targetPath)
        default:
            // Ignore other types
        }
    }
    return nil
}

// compatibility helper for the sequential path (used by older code path)
func downloadAndExtract(ctx context.Context, url, destDir string) error {
    if err := ensureDir(destDir); err != nil { return err }
    tmp := filepath.Join(destDir, ".pkg.tgz")
    if err := downloadToFileWithRetry(ctx, url, tmp, 3); err != nil { return err }
    if err := downloadAndExtractFromFile(tmp, destDir); err != nil { return err }
    return nil
}

// ---- pnpm-like store + symlink layout helpers ----

func defaultStoreDir() (string, error) {
    if v := os.Getenv("WLIM_STORE_DIR"); v != "" {
        return v, nil
    }
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(home, ".wlim", "store", "v3"), nil
}

// store path for a given package@version (no integrity yet)
func storePkgPath(storeDir, name, version string) string {
    // For simplicity: ~/.wlim/store/v3/<name>/<version>
    return filepath.Join(storeDir, name, version)
}

// ensure a symlink, replacing existing file/dir if necessary
func ensureSymlink(target, linkPath string) error {
    // Remove existing
    if fi, err := os.Lstat(linkPath); err == nil {
        if fi.Mode()&os.ModeSymlink != 0 || fi.IsDir() || fi.Mode().IsRegular() {
            _ = os.RemoveAll(linkPath)
        }
    }
    // Ensure parent
    if err := ensureDir(filepath.Dir(linkPath)); err != nil {
        return err
    }
    if err := os.Symlink(target, linkPath); err != nil {
        // Windows or restricted env: fall back to copy for dirs
        if runtime.GOOS == "windows" {
            // Best-effort directory copy fallback
            if fi, err2 := os.Stat(target); err2 == nil && fi.IsDir() {
                // simple recursive copy
                return copyDir(target, linkPath)
            }
        }
        return err
    }
    return nil
}

func copyDir(src, dst string) error {
    if err := ensureDir(dst); err != nil { return err }
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil { return err }
        rel, err := filepath.Rel(src, path)
        if err != nil { return err }
        if rel == "." { return nil }
        outPath := filepath.Join(dst, rel)
        if info.IsDir() {
            return ensureDir(outPath)
        }
        in, err := os.Open(path)
        if err != nil { return err }
        defer in.Close()
        if err := ensureDir(filepath.Dir(outPath)); err != nil { return err }
        out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
        if err != nil { return err }
        if _, err := io.Copy(out, in); err != nil { out.Close(); return err }
        return out.Close()
    })
}

// link into node_modules handling scopes
func linkIntoNodeModules(baseNodeModules, pkgName, target string) error {
    linkPath := filepath.Join(baseNodeModules, pkgName)
    return ensureSymlink(target, linkPath)
}

// read package.json to discover bin entries
type pkgJSON struct {
    Name string
    Bin  map[string]string
}

func readPackageJSON(dir string) (*pkgJSON, error) {
    f, err := os.Open(filepath.Join(dir, "package.json"))
    if err != nil {
        return nil, err
    }
    defer f.Close()
    var raw map[string]any
    if err := json.NewDecoder(f).Decode(&raw); err != nil {
        return nil, err
    }
    pj := &pkgJSON{Bin: make(map[string]string)}
    if n, ok := raw["name"].(string); ok { pj.Name = n }
    if b, ok := raw["bin"]; ok {
        switch v := b.(type) {
        case string:
            if pj.Name != "" { pj.Bin[pj.Name] = v }
        case map[string]any:
            for k, vv := range v {
                if s, ok := vv.(string); ok { pj.Bin[k] = s }
            }
        }
    }
    return pj, nil
}

func linkBins(projectDir, pkgDir string, pj *pkgJSON) error {
    if pj == nil || len(pj.Bin) == 0 {
        return nil
    }
    binDir := filepath.Join(projectDir, "node_modules", ".bin")
    if err := ensureDir(binDir); err != nil {
        return err
    }
    for binName, rel := range pj.Bin {
        src, err := safeJoin(pkgDir, rel)
        if err != nil {
            return err
        }
        dst := filepath.Join(binDir, binName)
        if err := ensureSymlink(src, dst); err != nil {
            return err
        }
    }
    return nil
}

func resolveAndInstall(ctx context.Context, packageName, versionSpec, projectDir string, installed map[string]bool, rootCache map[string]*RootDoc) error {
    v, md, err := resolveVersionAndMetadata(ctx, packageName, versionSpec, rootCache)
    if err != nil {
        return err
    }
    key := packageName + "@" + v
    if installed[key] {
        return nil
    }

    // 먼저 의존성 처리
    for depName, depSpec := range md.Dependencies {
        if err := resolveAndInstall(ctx, depName, depSpec, projectDir, installed, rootCache); err != nil {
            return err
        }
    }

    // pnpm-like store path
    storeDir, err := defaultStoreDir()
    if err != nil {
        return err
    }
    pkgStorePath := storePkgPath(storeDir, packageName, v)
    // Extract into store if not present
    if _, err := os.Stat(pkgStorePath); os.IsNotExist(err) {
        logf("Fetching %s@%s to store...\n", packageName, v)
        if err := ensureDir(pkgStorePath); err != nil {
            return err
        }
        if err := downloadAndExtract(ctx, md.Dist.Tarball, pkgStorePath); err != nil {
            return fmt.Errorf("failed to fetch %s@%s: %w", packageName, v, err)
        }
    }

    // Link package dependencies inside its own node_modules in store
    storeNM := filepath.Join(pkgStorePath, "node_modules")
    if err := ensureDir(storeNM); err != nil {
        return err
    }
    for depName := range md.Dependencies {
        // Resolve installed version to compute store path
        depV, _, err := resolveVersionAndMetadata(ctx, depName, md.Dependencies[depName], rootCache)
        if err != nil {
            return err
        }
        depStorePath := storePkgPath(storeDir, depName, depV)
        if err := linkIntoNodeModules(storeNM, depName, depStorePath); err != nil {
            return err
        }
    }

    // Link package into project node_modules
    projectNM := filepath.Join(projectDir, "node_modules")
    if err := linkIntoNodeModules(projectNM, packageName, pkgStorePath); err != nil {
        return err
    }

    // Link package bins for direct deps
    if pj, err := readPackageJSON(pkgStorePath); err == nil {
        _ = linkBins(projectDir, pkgStorePath, pj)
    }

    logf("Installed %s@%s\n", packageName, v)
    installed[key] = true
    return nil
}

// ---- Graph resolution and parallel fetching ----

type GraphNode struct {
    Name string
    Version string
    MD   *PackageMetadata
    Deps map[string]string // depName -> resolvedVersion
}

func keyOf(name, version string) string { return name + "@" + version }

func resolveGraph(ctx context.Context, rootName, rootSpec string, cache map[string]*RootDoc) (map[string]*GraphNode, *GraphNode, error) {
    // BFS/DFS hybrid with explicit queue of resolved nodes
    nodes := make(map[string]*GraphNode)
    // resolve root first
    v, md, err := resolveVersionAndMetadata(ctx, rootName, rootSpec, cache)
    if err != nil { return nil, nil, err }
    root := &GraphNode{Name: rootName, Version: v, MD: md, Deps: make(map[string]string)}
    nodes[keyOf(root.Name, root.Version)] = root
    type pair struct { name, version string }
    queue := []pair{{rootName, v}}

    for len(queue) > 0 {
        cur := queue[0]
        queue = queue[1:]
        curKey := keyOf(cur.name, cur.version)
        node := nodes[curKey]
        // compute deps resolved versions and enqueue
        for depName, spec := range node.MD.Dependencies {
            dv, dmd, err := resolveVersionAndMetadata(ctx, depName, spec, cache)
            if err != nil { return nil, nil, err }
            node.Deps[depName] = dv
            dkey := keyOf(depName, dv)
            if _, ok := nodes[dkey]; !ok {
                nodes[dkey] = &GraphNode{Name: depName, Version: dv, MD: dmd, Deps: make(map[string]string)}
                queue = append(queue, pair{depName, dv})
            }
        }
    }
    return nodes, root, nil
}

func parseSRI(integrity string) (algo string, sum []byte, ok bool) {
    if integrity == "" { return "", nil, false }
    // pick first sha512 entry if present
    parts := strings.Fields(integrity)
    for _, p := range parts {
        if strings.HasPrefix(p, "sha512-") {
            b64 := strings.TrimPrefix(p, "sha512-")
            bs, err := base64.StdEncoding.DecodeString(b64)
            if err == nil { return "sha512", bs, true }
        }
    }
    // fallback to first entry
    for _, p := range parts {
        if strings.HasPrefix(p, "sha1-") {
            b64 := strings.TrimPrefix(p, "sha1-")
            bs, err := base64.StdEncoding.DecodeString(b64)
            if err == nil { return "sha1", bs, true }
        }
    }
    return "", nil, false
}

func verifyIntegrityFile(path string, integrity, shasum string) error {
    if integrity != "" {
        algo, want, ok := parseSRI(integrity)
        if ok {
            f, err := os.Open(path)
            if err != nil { return err }
            defer f.Close()
            switch algo {
            case "sha512":
                h := sha512.New()
                if _, err := io.Copy(h, f); err != nil { return err }
                if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), hex.EncodeToString(want)) {
                    return errors.New("integrity mismatch (sha512)")
                }
                return nil
            case "sha1":
                h := sha1.New()
                if _, err := io.Copy(h, f); err != nil { return err }
                if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), hex.EncodeToString(want)) {
                    return errors.New("integrity mismatch (sha1)")
                }
                return nil
            }
        }
    }
    if shasum != "" { // npm legacy sha1 hex
        f, err := os.Open(path)
        if err != nil { return err }
        defer f.Close()
        h := sha1.New()
        if _, err := io.Copy(h, f); err != nil { return err }
        if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), strings.TrimSpace(shasum)) {
            return errors.New("integrity mismatch (shasum)")
        }
    }
    return nil
}

func downloadToFileWithRetry(ctx context.Context, url, outPath string, attempts int) error {
    // write to temp then rename
    tmp := outPath + ".partial"
    defer os.Remove(tmp)
    for i := 1; i <= attempts; i++ {
        resp, err := getWithRetry(ctx, url, 1)
        if err != nil {
            if i == attempts { return err }
            time.Sleep(time.Duration(i*i) * 200 * time.Millisecond)
            continue
        }
        f, err := os.Create(tmp)
        if err != nil { return err }
        _, copyErr := io.Copy(f, resp.Body)
        resp.Body.Close()
        cerr := f.Close()
        if copyErr == nil && cerr == nil {
            if err := os.Rename(tmp, outPath); err != nil { return err }
            return nil
        }
        if i == attempts {
            if copyErr != nil { return copyErr }
            return cerr
        }
        time.Sleep(time.Duration(i*i) * 200 * time.Millisecond)
    }
    return fmt.Errorf("failed to download %s", url)
}

type LockPackage struct {
    Name string `json:"name"`
    Version string `json:"version"`
    Dependencies map[string]string `json:"dependencies"`
}
type LockFile struct {
    Roots []string `json:"roots"`
    Packages map[string]LockPackage `json:"packages"`
}

func writeLockfile(projectDir string, roots []*GraphNode, nodes map[string]*GraphNode) error {
    lf := LockFile{Packages: make(map[string]LockPackage)}
    for _, r := range roots {
        lf.Roots = append(lf.Roots, keyOf(r.Name, r.Version))
    }
    for k, n := range nodes {
        lf.Packages[k] = LockPackage{Name: n.Name, Version: n.Version, Dependencies: n.Deps}
    }
    path := filepath.Join(projectDir, "wlim.lock")
    f, err := os.Create(path)
    if err != nil { return err }
    defer f.Close()
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    return enc.Encode(lf)
}

func readLockfile(projectDir string) (*LockFile, error) {
    b, err := os.ReadFile(filepath.Join(projectDir, "wlim.lock"))
    if err != nil { return nil, err }
    var lf LockFile
    if err := json.Unmarshal(b, &lf); err != nil { return nil, err }
    return &lf, nil
}

func metadataForExactVersion(ctx context.Context, name, version string, cache map[string]*RootDoc) (*PackageMetadata, error) {
    rd, err := fetchRootDoc(ctx, name, cache)
    if err != nil { return nil, err }
    md, ok := rd.Versions[version]
    if !ok { return nil, fmt.Errorf("version %s not found for %s", version, name) }
    copy := md
    return &copy, nil
}

// nodesFromLockfile builds nodes and roots from an existing lockfile
func nodesFromLockfile(ctx context.Context, projectDir string, cache map[string]*RootDoc) (map[string]*GraphNode, []*GraphNode, error) {
    lf, err := readLockfile(projectDir)
    if err != nil { return nil, nil, err }
    nodes := make(map[string]*GraphNode)
    for key, lp := range lf.Packages {
        md, err := metadataForExactVersion(ctx, lp.Name, lp.Version, cache)
        if err != nil { return nil, nil, err }
        nodes[key] = &GraphNode{Name: lp.Name, Version: lp.Version, MD: md, Deps: lp.Dependencies}
    }
    var roots []*GraphNode
    for _, r := range lf.Roots {
        if n, ok := nodes[r]; ok { roots = append(roots, n) }
    }
    if len(roots) == 0 { return nil, nil, fmt.Errorf("no roots in lockfile") }
    return nodes, roots, nil
}

// updateLockfile re-resolves the specified roots (or all, if names empty) to latest
// and writes a new lockfile capturing the new graph.
func updateLockfile(ctx context.Context, projectDir string, names []string, cache map[string]*RootDoc, policy string, specs map[string]string) error {
    lf, err := readLockfile(projectDir)
    if err != nil { return err }
    // Determine roots to update
    rootsToUpdate := make(map[string]bool)
    if len(names) == 0 {
        for _, r := range lf.Roots { // r is like name@version
            if at := strings.LastIndex(r, "@"); at > 0 { rootsToUpdate[r[:at]] = true }
        }
    } else {
        for _, n := range names { rootsToUpdate[n] = true }
    }
    // Resolve new graphs for those roots based on explicit specs or policy
    allNodes := make(map[string]*GraphNode)
    var roots []*GraphNode
    for _, r := range lf.Roots {
        name := r
        if at := strings.LastIndex(r, "@"); at > 0 { name = r[:at] }
        spec := "latest"
        if !rootsToUpdate[name] {
            // keep existing version spec from lockfile
            ver := r[strings.LastIndex(r, "@")+1:]
            spec = ver
        } else if s, ok := specs[name]; ok && s != "" {
            spec = s
        } else if policy == "minor" || policy == "patch" {
            // compute next version per policy vs current
            curVer := r[strings.LastIndex(r, "@")+1:]
            nv, err := nextVersionForPolicy(ctx, name, curVer, policy, cache)
            if err == nil && nv != "" {
                spec = nv
            } else {
                spec = "latest"
            }
        }
        nodes, root, err := resolveGraph(ctx, name, spec, cache)
        if err != nil { return err }
        for k, n := range nodes { allNodes[k] = n }
        roots = append(roots, root)
    }
    return writeLockfile(projectDir, roots, allNodes)
}

// nextVersionForPolicy picks the highest version according to policy compared to current
func nextVersionForPolicy(ctx context.Context, name, current, policy string, cache map[string]*RootDoc) (string, error) {
    rd, err := fetchRootDoc(ctx, name, cache)
    if err != nil { return "", err }
    cur, err := semver.NewVersion(current)
    if err != nil { return "", err }
    var best *semver.Version
    for vStr := range rd.Versions {
        v, err := semver.NewVersion(vStr)
        if err != nil { continue }
        // Must be >= current
        if v.LessThan(cur) || v.Equal(cur) { continue }
        switch policy {
        case "patch":
            if v.Major() != cur.Major() || v.Minor() != cur.Minor() { continue }
        case "minor":
            if v.Major() != cur.Major() { continue }
        default:
            // latest: no additional constraint
        }
        if best == nil || best.LessThan(v) {
            vv := v
            best = vv
        }
    }
    if best == nil { return current, nil }
    return best.Original(), nil
}

// cleanStore removes store entries not present in the current lockfile.
func cleanStore(projectDir, storeDir string, dryRun bool) error {
    lf, err := readLockfile(projectDir)
    if err != nil { return err }
    referenced := make(map[string]bool)
    for k := range lf.Packages { // keys are name@version
        parts := strings.SplitN(k, "@", 2)
        if len(parts) != 2 { continue }
        referenced[filepath.Join(storeDir, parts[0], parts[1])] = true
    }
    // Walk storeDir
    return filepath.Walk(storeDir, func(path string, info os.FileInfo, err error) error {
        if err != nil { return err }
        if !info.IsDir() { return nil }
        // Only act on directories at depth >=2
        rel, err := filepath.Rel(storeDir, path)
        if err != nil || rel == "." { return nil }
        parts := strings.Split(rel, string(os.PathSeparator))
        if len(parts) != 2 { return nil }
        if !referenced[path] {
            if !dryRun {
                return os.RemoveAll(path)
            }
        }
        return nil
    })
}

// listLockfile returns roots and packages from the lockfile
func listLockfile(projectDir string) ([]string, map[string]LockPackage, error) {
    lf, err := readLockfile(projectDir)
    if err != nil { return nil, nil, err }
    return lf.Roots, lf.Packages, nil
}

// removeFromLockfile removes given roots and prunes unreachable packages
func removeFromLockfile(projectDir string, names []string) error {
    lf, err := readLockfile(projectDir)
    if err != nil { return err }
    // remove roots by name
    toRemove := make(map[string]bool)
    for _, n := range names { toRemove[n] = true }
    var keptRoots []string
    for _, r := range lf.Roots {
        name := r
        if at := strings.LastIndex(r, "@"); at > 0 { name = r[:at] }
        if !toRemove[name] { keptRoots = append(keptRoots, r) }
    }
    lf.Roots = keptRoots
    // compute reachable package keys from keptRoots
    reachable := make(map[string]bool)
    // build adjacency from lockfile packages
    adj := make(map[string][]string) // key -> list of dep keys
    for key, pkg := range lf.Packages {
        var deps []string
        for depName, depVer := range pkg.Dependencies {
            deps = append(deps, depName+"@"+depVer)
        }
        adj[key] = deps
    }
    // seed queue with kept roots
    queue := []string{}
    for _, r := range lf.Roots { queue = append(queue, r) }
    for len(queue) > 0 {
        k := queue[0]
        queue = queue[1:]
        if reachable[k] { continue }
        reachable[k] = true
        for _, d := range adj[k] { queue = append(queue, d) }
    }
    // prune packages not reachable
    keptPkgs := make(map[string]LockPackage)
    for k, v := range lf.Packages {
        if reachable[k] { keptPkgs[k] = v }
    }
    lf.Packages = keptPkgs
    // write back
    path := filepath.Join(projectDir, "wlim.lock")
    f, err := os.Create(path)
    if err != nil { return err }
    defer f.Close()
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    return enc.Encode(lf)
}

func installParallel(ctx context.Context, projectDir, storeDir string, root *GraphNode, nodes map[string]*GraphNode, concurrency int) error {
    type task struct {
        node *GraphNode
    }
    tasks := make(chan task)
    errCh := make(chan error, concurrency)
    done := make(chan struct{})

    // workers: fetch+extract if needed
    total := len(nodes)
    var doneCount int64
    for i := 0; i < concurrency; i++ {
        go func() {
            for t := range tasks {
                n := t.node
                pkgStorePath := storePkgPath(storeDir, n.Name, n.Version)
                if _, err := os.Stat(pkgStorePath); os.IsNotExist(err) {
                    vStage("fetch", n.Name, n.Version)
                    logf("Downloading %s@%s\n", n.Name, n.Version)
                    if err := ensureDir(pkgStorePath); err != nil { select { case errCh <- err: default: }; continue }
                    tarPath := filepath.Join(pkgStorePath, "pkg.tgz")
                    // download
                    if err := downloadToFileWithRetry(ctx, n.MD.Dist.Tarball, tarPath, 3); err != nil { select { case errCh <- err: default: }; continue }
                    if err := verifyIntegrityFile(tarPath, n.MD.Dist.Integrity, n.MD.Dist.Shasum); err != nil { select { case errCh <- err: default: }; continue }
                    vStage("extract", n.Name, n.Version)
                    if err := downloadAndExtractFromFile(tarPath, pkgStorePath); err != nil { select { case errCh <- err: default: }; continue }
                    // keep tarball for cache; optional: os.Remove(tarPath)
                }
                // link deps inside store
                storeNM := filepath.Join(pkgStorePath, "node_modules")
                if err := ensureDir(storeNM); err != nil { select { case errCh <- err: default: }; continue }
                vStage("link-deps", n.Name, n.Version)
                for depName, depV := range n.Deps {
                    depStore := storePkgPath(storeDir, depName, depV)
                    if err := linkIntoNodeModules(storeNM, depName, depStore); err != nil { select { case errCh <- err: default: }; break }
                }
                // progress
                if total > 0 {
                    doneCount++
                    logf("%d/%d %s@%s\n", doneCount, total, n.Name, n.Version)
                }
            }
            done <- struct{}{}
        }()
    }

    go func() {
        for _, n := range nodes {
            tasks <- task{node: n}
        }
        close(tasks)
    }()

    // wait workers
    for i := 0; i < concurrency; i++ { <-done }
    close(done)
    select {
    case err := <-errCh:
        return err
    default:
    }

    // link root into project and bins
    projectNM := filepath.Join(projectDir, "node_modules")
    vStage("link-root", root.Name, root.Version)
    if err := linkIntoNodeModules(projectNM, root.Name, storePkgPath(storeDir, root.Name, root.Version)); err != nil {
        return err
    }
    if pj, err := readPackageJSON(storePkgPath(storeDir, root.Name, root.Version)); err == nil {
        _ = linkBins(projectDir, storePkgPath(storeDir, root.Name, root.Version), pj)
    }
    vStage("done", root.Name, root.Version)
    return nil
}

// installGraph installs all nodes in parallel once, then links all roots into project and bins.
func installGraph(ctx context.Context, projectDir, storeDir string, roots []*GraphNode, nodes map[string]*GraphNode, concurrency int) error {
    // Reuse installParallel workers but avoid duplicate fetch by marking existence
    type task struct{ node *GraphNode }
    tasks := make(chan task)
    errCh := make(chan error, concurrency)
    done := make(chan struct{})

    for i := 0; i < concurrency; i++ {
        go func() {
            for t := range tasks {
                n := t.node
                pkgStorePath := storePkgPath(storeDir, n.Name, n.Version)
                if _, err := os.Stat(pkgStorePath); os.IsNotExist(err) {
                    vStage("fetch", n.Name, n.Version)
                    logf("Downloading %s@%s\n", n.Name, n.Version)
                    if err := ensureDir(pkgStorePath); err != nil { select { case errCh <- err: default: }; continue }
                    tarPath := filepath.Join(pkgStorePath, "pkg.tgz")
                    if err := downloadToFileWithRetry(ctx, n.MD.Dist.Tarball, tarPath, 3); err != nil { select { case errCh <- err: default: }; continue }
                    if err := verifyIntegrityFile(tarPath, n.MD.Dist.Integrity, n.MD.Dist.Shasum); err != nil { select { case errCh <- err: default: }; continue }
                    vStage("extract", n.Name, n.Version)
                    if err := downloadAndExtractFromFile(tarPath, pkgStorePath); err != nil { select { case errCh <- err: default: }; continue }
                }
                storeNM := filepath.Join(pkgStorePath, "node_modules")
                if err := ensureDir(storeNM); err != nil { select { case errCh <- err: default: }; continue }
                vStage("link-deps", n.Name, n.Version)
                for depName, depV := range n.Deps {
                    depStore := storePkgPath(storeDir, depName, depV)
                    if err := linkIntoNodeModules(storeNM, depName, depStore); err != nil { select { case errCh <- err: default: }; break }
                }
            }
            done <- struct{}{}
        }()
    }
    go func() {
        for _, n := range nodes { tasks <- task{node:n} }
        close(tasks)
    }()
    for i:=0;i<concurrency;i++ { <-done }
    select{
    case err := <-errCh:
        return err
    default:
    }
    // link roots and bins
    for _, r := range roots {
        vStage("link-root", r.Name, r.Version)
        if err := linkIntoNodeModules(filepath.Join(projectDir, "node_modules"), r.Name, storePkgPath(storeDir, r.Name, r.Version)); err != nil {
            return err
        }
        if pj, err := readPackageJSON(storePkgPath(storeDir, r.Name, r.Version)); err == nil {
            _ = linkBins(projectDir, storePkgPath(storeDir, r.Name, r.Version), pj)
        }
        vStage("done", r.Name, r.Version)
    }
    return nil
}

// ---- Disk metadata cache for registry root docs ----
func cacheBaseDir() (string, error) {
    if v := os.Getenv("WLIM_CACHE_DIR"); v != "" { return v, nil }
    home, err := os.UserHomeDir()
    if err != nil { return "", err }
    return filepath.Join(home, ".wlim", "cache"), nil
}

func rootDocCachePath(pkg string) (string, error) {
    base, err := cacheBaseDir()
    if err != nil { return "", err }
    return filepath.Join(base, "registry", pkg+".json"), nil
}

func readRootDocCache(pkg string) (*RootDoc, error) {
    p, err := rootDocCachePath(pkg)
    if err != nil { return nil, err }
    b, err := os.ReadFile(p)
    if err != nil { return nil, err }
    var rd RootDoc
    if err := json.Unmarshal(b, &rd); err != nil { return nil, err }
    return &rd, nil
}

func writeRootDocCache(pkg string, rd *RootDoc) error {
    p, err := rootDocCachePath(pkg)
    if err != nil { return err }
    if err := ensureDir(filepath.Dir(p)); err != nil { return err }
    b, err := json.Marshal(rd)
    if err != nil { return err }
    return os.WriteFile(p, b, 0o644)
}

func cacheTTLSeconds() int64 {
    if v := os.Getenv("WLIM_CACHE_TTL_SECONDS"); v != "" {
        if n, err := strconv.ParseInt(v, 10, 64); err == nil { return n }
    }
    return -1 // no TTL
}

func tryReadRootDocCacheWithTTL(pkg string) (bool, *RootDoc) {
    p, err := rootDocCachePath(pkg)
    if err != nil { return false, nil }
    fi, err := os.Stat(p)
    if err != nil { return false, nil }
    ttl := cacheTTLSeconds()
    if ttl == 0 {
        return false, nil
    }
    if ttl > 0 {
        if time.Since(fi.ModTime()) > time.Duration(ttl)*time.Second {
            return false, nil
        }
    }
    rd, err := readRootDocCache(pkg)
    if err != nil { return false, nil }
    return true, rd
}

func logf(format string, a ...interface{}) {
    if verbose { fmt.Printf(format, a...) }
}

// installCmd 명령어 정의
var installCmd = &cobra.Command{
    Use:   "install [<package>[@version|@range] ...]",
    Short: "Install one or more packages, or from lockfile",
    Args:  cobra.ArbitraryArgs,
    Run: func(cmd *cobra.Command, args []string) {
        projectDir, _ := cmd.Flags().GetString("dir")
        if projectDir == "" {
            projectDir = "."
        }
        cfg, _ := loadConfig(projectDir)
        // store dir
        storeOverride, _ := cmd.Flags().GetString("store-dir")
        if storeOverride == "" && cfg.StoreDir != "" {
            storeOverride = cfg.StoreDir
        }
        if storeOverride != "" {
            os.Setenv("WICK_STORE_DIR", storeOverride)
        }
        // registry flag overrides env
        if r, _ := cmd.Flags().GetString("registry"); r != "" {
            registryOverride = r
        } else if cfg.Registry != "" {
            registryOverride = cfg.Registry
        }
        frozen, _ := cmd.Flags().GetBool("frozen-lockfile")

        ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
        defer cancel()
        cache := make(map[string]*RootDoc)
        var (
            allNodes map[string]*GraphNode
            roots []*GraphNode
        )
        if len(args) == 0 || frozen {
            // Lockfile-driven install
            var err error
            allNodes, roots, err = nodesFromLockfile(ctx, projectDir, cache)
            if err != nil {
                fmt.Println("Error:", err)
                os.Exit(1)
            }
            if len(args) > 0 {
                // If frozen and explicit args present, ensure each root exists in lockfile
                namesInLock := map[string]bool{}
                for _, r := range roots { namesInLock[r.Name] = true }
                for _, a := range args {
                    name := a
                    if at := strings.LastIndex(a, "@"); at > 0 { name = a[:at] }
                    if !namesInLock[name] {
                        fmt.Printf("Error: %s not present in lockfile (frozen)\n", name)
                        os.Exit(1)
                    }
                }
            }
        } else {
            // Resolve graphs for each arg and merge the node set
            allNodes = make(map[string]*GraphNode)
            for _, arg := range args {
                pkg := arg
                spec := "latest"
                if at := strings.LastIndex(arg, "@"); at > 0 {
                    pkg = arg[:at]
                    spec = arg[at+1:]
                    if spec == "" { spec = "latest" }
                }
                nodes, root, err := resolveGraph(ctx, pkg, spec, cache)
                if err != nil {
                    fmt.Println("Error:", err)
                    os.Exit(1)
                }
                for k, n := range nodes { allNodes[k] = n }
                roots = append(roots, root)
            }
        }
        // Install in parallel and link each root
        storeDir, err := defaultStoreDir()
        if err != nil {
            fmt.Println("Error:", err)
            os.Exit(1)
        }
        conc, _ := cmd.Flags().GetInt("concurrency")
        if !cmd.Flags().Changed("concurrency") && cfg.Concurrency > 0 {
            conc = cfg.Concurrency
        }
        if conc <= 0 { conc = runtime.NumCPU() }
        // visual flags
        if f, _ := cmd.Flags().GetString("log-format"); f != "" { logFormat = f }
        logNoColor, _ = cmd.Flags().GetBool("no-color")
        showProgress, _ = cmd.Flags().GetBool("progress")
        if err := installGraph(ctx, projectDir, storeDir, roots, allNodes, conc); err != nil {
            fmt.Println("Error:", err)
            os.Exit(1)
        }
        // Write lockfile
        if err := writeLockfile(projectDir, roots, allNodes); err != nil {
            fmt.Println("Warning: failed to write lockfile:", err)
        }
        fmt.Println("Done.")
    },
}

func init() {
    installCmd.Flags().String("dir", ".", "Project directory where node_modules resides")
    installCmd.Flags().String("store-dir", "", "Override content-addressable store directory (defaults to ~/.wlim/store/v3 or WLIM_STORE_DIR)")
    installCmd.Flags().Int("concurrency", runtime.NumCPU(), "Parallel downloads/extract workers")
    installCmd.Flags().String("registry", "", "Override npm registry base URL (takes precedence over WLIM_REGISTRY)")
    installCmd.Flags().Bool("frozen-lockfile", false, "Use existing wlim.lock exclusively and fail on mismatches")
    installCmd.Flags().String("log-format", "fancy", "Log format: fancy|plain")
    installCmd.Flags().Bool("no-color", false, "Disable ANSI colors in logs")
    installCmd.Flags().Bool("progress", true, "Show progress bar")
    rootCmd.AddCommand(installCmd)
}
