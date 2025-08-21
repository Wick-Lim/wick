/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
    "os"

    "github.com/spf13/cobra"
)

var verbose bool


// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
    Use:   "wick",
    Short: "Fast npm-like package installer",
    Long:  "wick is a minimal npm-like installer written in Go. It resolves semver ranges, fetches tarballs from the npm registry, and installs into node_modules.",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
    // Global flags
    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
}
