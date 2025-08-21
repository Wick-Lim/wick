package cmd

import (
  "context"
  "fmt"
  "os"
  "runtime"
  "time"
  "strings"

  "github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
  Use:   "update [<package> ...]",
  Short: "Update lockfile for roots and reinstall",
  Args:  cobra.ArbitraryArgs,
  Run: func(cmd *cobra.Command, args []string) {
    projectDir, _ := cmd.Flags().GetString("dir")
    if projectDir == "" { projectDir = "." }
    cfg, _ := loadConfig(projectDir)
    // registry override
    if r, _ := cmd.Flags().GetString("registry"); r != "" { registryOverride = r }
    if registryOverride == "" && cfg.Registry != "" { registryOverride = cfg.Registry }

    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel()
    cache := make(map[string]*RootDoc)
    policy, _ := cmd.Flags().GetString("policy")
    // parse explicit specs from args like name@spec
    specs := make(map[string]string)
    names := make([]string, 0, len(args))
    for _, a := range args {
      name := a
      if at := strings.LastIndex(a, "@"); at > 0 {
        name = a[:at]
        specs[name] = a[at+1:]
      }
      names = append(names, name)
    }
    if err := updateLockfile(ctx, projectDir, names, cache, policy, specs); err != nil {
      fmt.Println("Error:", err)
      os.Exit(1)
    }
    // After updating lockfile, install from it
    nodes, roots, err := nodesFromLockfile(ctx, projectDir, cache)
    if err != nil {
      fmt.Println("Error:", err)
      os.Exit(1)
    }
    storeDir, err := defaultStoreDir()
    if err != nil { fmt.Println("Error:", err); os.Exit(1) }
    conc, _ := cmd.Flags().GetInt("concurrency")
    if !cmd.Flags().Changed("concurrency") && cfg.Concurrency > 0 { conc = cfg.Concurrency }
    if conc <= 0 { conc = runtime.NumCPU() }
    for _, r := range roots {
      if err := installParallel(ctx, projectDir, storeDir, r, nodes, conc); err != nil {
        fmt.Println("Error:", err)
        os.Exit(1)
      }
    }
    fmt.Println("Updated and installed.")
  },
}

func init() {
  updateCmd.Flags().String("dir", ".", "Project directory where node_modules resides")
  updateCmd.Flags().Int("concurrency", runtime.NumCPU(), "Parallel downloads/extract workers")
  updateCmd.Flags().String("registry", "", "Override npm registry base URL (takes precedence over WICK_REGISTRY)")
  updateCmd.Flags().String("policy", "latest", "Update policy: latest|minor|patch")
  rootCmd.AddCommand(updateCmd)
}
