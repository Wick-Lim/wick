package cmd

import (
  "fmt"
  "github.com/spf13/cobra"
  iv "github.com/wicklim/wlim/internal/version"
)

var versionCmd = &cobra.Command{
  Use:   "version",
  Short: "Print wlim version",
  Run: func(cmd *cobra.Command, args []string) {
    fmt.Println(iv.Version)
  },
}

func init() {
  rootCmd.AddCommand(versionCmd)
}
