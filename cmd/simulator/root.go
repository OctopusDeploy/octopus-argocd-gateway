package main

import (
	"github.com/spf13/cobra"
)

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "simulator",
		Short: "Load simulator for the Octopus Deploy ArgoCD Gateway",
		Long: `simulator emulates many virtual gateway connections to an Octopus Server.

Use the run subcommand to execute a single performance test scenario
against a server. The simulator opens the same seven gRPC streams a real
gateway opens for each virtual gateway, drives the scenario workload,
and writes a JSON report at exit.`,
	}

	root.AddCommand(newRunCommand())
	return root
}
