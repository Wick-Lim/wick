package cmd

import (
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
  Use:   "validate",
  Short: "Validate lockfile and store/project consistency",
  Run: func(cmd *cobra.Command, args []string) {
    projectDir, _ := cmd.Flags().GetString("dir")
    if projectDir == "" { projectDir = "." }
    if err := validateProject(projectDir); err != nil {
      fmt.Println("Invalid:", err)
      os.Exit(1)
    }
    fmt.Println("Valid.")
  },
}

func validateProject(projectDir string) error {
  lf, err := readLockfile(projectDir)
  if err != nil { return err }
  storeDir, err := defaultStoreDir()
  if err != nil { return err }
  // roots must be linked in project
  for _, r := range lf.Roots {
    name := r
    if at := strings.LastIndex(r, "@"); at > 0 { name = r[:at] }
    // handle scoped path
    link := filepath.Join(projectDir, "node_modules")
    if strings.HasPrefix(name, "@") {
      parts := strings.SplitN(name, "/", 2)
      if len(parts) == 2 { link = filepath.Join(link, parts[0], parts[1]) } else { link = filepath.Join(link, name) }
    } else {
      link = filepath.Join(link, name)
    }
    if _, err := os.Lstat(link); err != nil { return fmt.Errorf("missing root link: %s", name) }
  }
  // store entries exist
  for k := range lf.Packages {
    parts := strings.SplitN(k, "@", 2)
    if len(parts) != 2 { continue }
    if _, err := os.Stat(filepath.Join(storeDir, parts[0], parts[1])); err != nil { return fmt.Errorf("missing store entry: %s", k) }
  }
  return nil
}

func init() {
  validateCmd.Flags().String("dir", ".", "Project directory where node_modules resides")
  rootCmd.AddCommand(validateCmd)
}
