package cmd

import (
  "fmt"
  "github.com/spf13/cobra"
  iv "github.com/wicklim/wick/internal/version"
)

var versionCmd = &cobra.Command{
  Use:   "version",
  Short: "Print wick version",
  Run: func(cmd *cobra.Command, args []string) {
    fmt.Println(iv.Version)
  },
}

func init() {
  rootCmd.AddCommand(versionCmd)
}
