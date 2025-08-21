package cmd

import (
  "fmt"
  "os"
  "github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
  Use:   "init",
  Short: "Create a default wlim.json in the project",
  Run: func(cmd *cobra.Command, args []string) {
    projectDir, _ := cmd.Flags().GetString("dir")
    if projectDir == "" { projectDir = "." }
    reg, _ := cmd.Flags().GetString("registry")
    store, _ := cmd.Flags().GetString("store-dir")
    conc, _ := cmd.Flags().GetInt("concurrency")
    cfg := &Config{Registry: reg, StoreDir: store, Concurrency: conc}
    if err := initProjectConfig(projectDir, cfg); err != nil {
      fmt.Println("Error:", err)
      os.Exit(1)
    }
    fmt.Println("Created wlim.json")
  },
}

func init() {
  initCmd.Flags().String("dir", ".", "Project directory")
  initCmd.Flags().String("registry", "", "Default registry URL")
  initCmd.Flags().String("store-dir", "", "Default store directory")
  initCmd.Flags().Int("concurrency", 0, "Default concurrency")
  rootCmd.AddCommand(initCmd)
}
