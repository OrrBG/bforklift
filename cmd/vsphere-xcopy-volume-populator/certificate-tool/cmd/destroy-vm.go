package cmd

import (
	"certificate-tool/pkg/vmware"
	"time"

	"github.com/spf13/cobra"
)

var destroyVMCmd = &cobra.Command{
	Use: "destroy-vm",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use values from appConfig
		return vmware.DestroyVM(
			appConfig.VmName,
			appConfig.VsphereURL,
			appConfig.VsphereUser,
			appConfig.VspherePassword,
			appConfig.DataCenter,
			appConfig.DataStore,
			appConfig.Pool,
			parseDuration(appConfig.WaitTimeout, 5*time.Minute),
		)
	},
}

func init() {
	RootCmd.AddCommand(destroyVMCmd)
}
