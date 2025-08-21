package cmd

import (
  "encoding/json"
  "fmt"
  "os"
  "sort"
  "github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
  Use:   "list",
  Short: "List roots and packages from wlim.lock",
  Run: func(cmd *cobra.Command, args []string) {
    projectDir, _ := cmd.Flags().GetString("dir")
    if projectDir == "" { projectDir = "." }
    roots, pkgs, err := listLockfile(projectDir)
    if err != nil { fmt.Println("Error:", err); return }
    format, _ := cmd.Flags().GetString("format")
    asJSON, _ := cmd.Flags().GetBool("json")
    if asJSON || format == "json" {
      if err := printLockfileJSON(roots, pkgs); err != nil { fmt.Println("Error:", err) }
      return
    } else if format == "yaml" {
      if err := printLockfileYAML(roots, pkgs); err != nil { fmt.Println("Error:", err) }
      return
    }
    fmt.Println("Roots:")
    rs := make([]string, len(roots))
    copy(rs, roots)
    sort.Strings(rs)
    for _, r := range rs { fmt.Println(" ", r) }
    fmt.Println("Packages:")
    keys := make([]string, 0, len(pkgs))
    for k := range pkgs { keys = append(keys, k) }
    sort.Strings(keys)
    for _, k := range keys { fmt.Println(" ", k) }
  },
}

func init() {
  listCmd.Flags().String("dir", ".", "Project directory where node_modules resides")
  listCmd.Flags().Bool("json", false, "Output lockfile contents as JSON")
  listCmd.Flags().String("format", "table", "Output format: table|json|yaml")
  rootCmd.AddCommand(listCmd)
}

// printLockfileJSON prints a minimal JSON for list output
func printLockfileJSON(roots []string, pkgs map[string]LockPackage) error {
  type out struct {
    Roots []string `json:"roots"`
    Packages map[string]LockPackage `json:"packages"`
  }
  o := out{Roots: roots, Packages: pkgs}
  enc := json.NewEncoder(os.Stdout)
  enc.SetIndent("", "  ")
  return enc.Encode(o)
}

// printLockfileYAML prints a simple YAML representation without external deps
func printLockfileYAML(roots []string, pkgs map[string]LockPackage) error {
  // roots
  fmt.Fprintln(os.Stdout, "roots:")
  for _, r := range roots {
    fmt.Fprintf(os.Stdout, "- %s\n", r)
  }
  // packages
  fmt.Fprintln(os.Stdout, "packages:")
  keys := make([]string, 0, len(pkgs))
  for k := range pkgs { keys = append(keys, k) }
  sort.Strings(keys)
  for _, k := range keys {
    p := pkgs[k]
    fmt.Fprintf(os.Stdout, "  %s:\n", k)
    fmt.Fprintf(os.Stdout, "    name: %s\n", p.Name)
    fmt.Fprintf(os.Stdout, "    version: %s\n", p.Version)
    if len(p.Dependencies) > 0 {
      fmt.Fprintln(os.Stdout, "    dependencies:")
      // stable order
      dk := make([]string, 0, len(p.Dependencies))
      for dn := range p.Dependencies { dk = append(dk, dn) }
      sort.Strings(dk)
      for _, dn := range dk {
        fmt.Fprintf(os.Stdout, "      %s: %s\n", dn, p.Dependencies[dn])
      }
    }
  }
  return nil
}
