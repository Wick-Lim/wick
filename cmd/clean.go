package cmd

import (
  "fmt"
  "os"
  "github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
  Use:   "clean",
  Short: "Remove unreferenced packages from the store per lockfile",
  Run: func(cmd *cobra.Command, args []string) {
    projectDir, _ := cmd.Flags().GetString("dir")
    if projectDir == "" { projectDir = "." }
    storeDir, _ := defaultStoreDir()
    dry, _ := cmd.Flags().GetBool("dry-run")
    if err := cleanStore(projectDir, storeDir, dry); err != nil {
      fmt.Println("Error:", err)
      os.Exit(1)
    }
    if dry { fmt.Println("Clean dry-run complete.") } else { fmt.Println("Clean complete.") }
  },
}

func init() {
  cleanCmd.Flags().String("dir", ".", "Project directory where node_modules resides")
  cleanCmd.Flags().Bool("dry-run", false, "Only print actions (no deletions)")
  rootCmd.AddCommand(cleanCmd)
}

