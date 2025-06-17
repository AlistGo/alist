//go:build !windows

package cmd

import (
	"os"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// StopCmd represents the stop command
var StopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop alist server by daemon/pid file",
	Run: func(cmd *cobra.Command, args []string) {
		stop()
	},
}

func stop() {
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
	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Errorf("failed to terminate process %d: %v", pid, err)
	} else {
		log.Info("terminated process: ", pid)
	}
	err = os.Remove(pidFile)
	if err != nil {
		log.Errorf("failed to remove pid file")
	}
	pid = -1
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
