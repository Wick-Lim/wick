package cmd

import (
  "fmt"
  "os"
  "path/filepath"

  "github.com/spf13/cobra"
)

// removeFromProject removes package links and its bin links from the project.
func removeFromProject(projectDir string, pkgs []string) error {
  nm := filepath.Join(projectDir, "node_modules")
  // remove package links
  for _, p := range pkgs {
    _ = os.RemoveAll(filepath.Join(nm, p))
  }
  // remove bins that match package names
  binDir := filepath.Join(nm, ".bin")
  for _, p := range pkgs {
    _ = os.RemoveAll(filepath.Join(binDir, p))
  }
  return nil
}

var removeCmd = &cobra.Command{
  Use:   "remove <package> [...]",
  Short: "Remove packages from project node_modules (keeps store)",
  Args:  cobra.MinimumNArgs(1),
  Run: func(cmd *cobra.Command, args []string) {
    projectDir, _ := cmd.Flags().GetString("dir")
    if projectDir == "" { projectDir = "." }
    if err := removeFromProject(projectDir, args); err != nil {
      fmt.Println("Error:", err)
      os.Exit(1)
    }
    // Update lockfile to drop roots and prune unreachable
    if err := removeFromLockfile(projectDir, args); err != nil {
      fmt.Println("Warning: failed to update lockfile:", err)
    }
    if clean, _ := cmd.Flags().GetBool("clean-store"); clean {
      storeDir, _ := defaultStoreDir()
      if err := cleanStore(projectDir, storeDir, false); err != nil {
        fmt.Println("Warning: failed to clean store:", err)
      }
    }
    fmt.Println("Removed:", args)
  },
}

func init() {
  removeCmd.Flags().String("dir", ".", "Project directory where node_modules resides")
  removeCmd.Flags().Bool("clean-store", false, "Also remove unreferenced entries from the store per lockfile")
  rootCmd.AddCommand(removeCmd)
}
