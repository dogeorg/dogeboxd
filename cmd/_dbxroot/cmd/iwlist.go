package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var iwlistCmd = &cobra.Command{
	Use:   "iwlist",
	Short: "Run iwlist commands to scan for wireless networks",
	Run: func(cmd *cobra.Command, args []string) {
		iwlistCmd := exec.Command("iwlist", args...)
		multiWriter := io.MultiWriter(os.Stdout)
		iwlistCmd.Stderr = multiWriter
		iwlistCmd.Stdout = multiWriter
		err := iwlistCmd.Run()
		if err != nil {
			fmt.Printf("Error running iwlist command: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(iwlistCmd)
}
