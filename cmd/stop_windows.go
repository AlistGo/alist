//go:build windows

package cmd

import (
	"github.com/spf13/cobra"
)

// StopCmd represents the stop command
var StopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Same as the kill command",
	Run: func(cmd *cobra.Command, args []string) {
		stop()
	},
}

func stop() {
	kill()
}

func init() {
	RootCmd.AddCommand(StopCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// stopCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// stopCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
