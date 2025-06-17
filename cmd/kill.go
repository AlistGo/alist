package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// KillCmd represents the kill command
var KillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Force kill alist server process by daemon/pid file",
	Run: func(cmd *cobra.Command, args []string) {
		kill()
	},
}

func kill() {
	initDaemon()
	if pid == -1 {
		log.Info("Seems not have been started. Try use `alist start` to start server.")
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		log.Errorf("failed to find process by pid: %d, reason: %v", pid, process)
		return
	}
	err = process.Kill()
	if err != nil {
		log.Errorf("failed to kill process %d: %v", pid, err)
	} else {
		log.Info("killed process: ", pid)
	}
	err = os.Remove(pidFile)
	if err != nil {
		log.Errorf("failed to remove pid file")
	}
	pid = -1
}

func init() {
	RootCmd.AddCommand(KillCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// stopCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// stopCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
