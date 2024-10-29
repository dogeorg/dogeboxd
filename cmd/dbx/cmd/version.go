package cmd

import (
	"fmt"

	"github.com/dogeorg/dogeboxd/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Get dogebox version information",
	Run: func(cmd *cobra.Command, args []string) {
		version := version.GetDBXRelease()

		fmt.Printf("Release: %s\n", version.Release)
		fmt.Printf("NurHash: %s\n", version.NurHash)
		fmt.Printf("Git: %s\n", version.Git.Commit)
		fmt.Printf("Dirty: %t\n", version.Git.Dirty)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
