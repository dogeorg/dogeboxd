package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dbx",
	Short: "dbx is used to interact with your dogebox and for development",
	Long:  `dbx is used to interact with your dogebox and for development`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

}
