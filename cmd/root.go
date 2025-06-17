package cmd

import (
	"fmt"
	"os"

	"github.com/alist-org/alist/v3/cmd/flags"
	_ "github.com/alist-org/alist/v3/drivers"
	_ "github.com/alist-org/alist/v3/internal/archive"
	_ "github.com/alist-org/alist/v3/internal/offline_download"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "alist",
	Short: "A file list program that supports multiple storage.",
	Long: `A file list program that supports multiple storage,
built with love by Xhofe and friends in Go/Solid.js.
Complete documentation is available at https://alist.nn.ci/`,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flags.DataDir, "data", "data", "data folder")
	RootCmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "start with debug mode")
	RootCmd.PersistentFlags().BoolVar(&flags.NoPrefix, "no-prefix", false, "disable env prefix")
	RootCmd.PersistentFlags().BoolVar(&flags.Dev, "dev", false, "start with dev mode")
	RootCmd.PersistentFlags().BoolVar(&flags.ForceBinDir, "force-bin-dir", false, "Force to use the directory where the binary file is located as data directory")
	RootCmd.PersistentFlags().BoolVar(&flags.LogStd, "log-std", false, "Force to log to std")
}
