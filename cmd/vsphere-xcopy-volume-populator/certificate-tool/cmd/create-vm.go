package cmd

import (
	"certificate-tool/pkg/vmware"
	"time"

	"github.com/spf13/cobra"
)

var createVmCmd = &cobra.Command{
	Use:   "create-vm",
	Short: "Create a VM, attach ISO, and inject data into guest",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use values from appConfig
		waitTimeoutDuration := parseDuration(appConfig.WaitTimeout, 10*time.Minute)
		_, err := vmware.CreateVM(
			appConfig.VmName,
			appConfig.VsphereURL,
			appConfig.VsphereUser,
			appConfig.VspherePassword,
			appConfig.DataCenter,
			appConfig.DataStore,
			appConfig.Pool,
			appConfig.DownloadVmdkURL,
			appConfig.LocalVmdkPath,
			appConfig.IsoPath,
			waitTimeoutDuration,
		)
		return err
	},
}

func init() {
	RootCmd.AddCommand(createVmCmd)
}
