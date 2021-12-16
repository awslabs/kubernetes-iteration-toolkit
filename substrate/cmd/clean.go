package main

import (
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up the environment setup",
	Long:  ``,
	Run:   Clean,
}

func Clean(cmd *cobra.Command, args []string) {
}

func init() {
	rootCmd.LocalFlags().StringVar(&options.File, "name", "", "Name for the environment")
	rootCmd.LocalFlags().StringVarP(&options.File, "file", "f", "", "Configuration file for the environment")
	rootCmd.AddCommand(cleanCmd)
}
