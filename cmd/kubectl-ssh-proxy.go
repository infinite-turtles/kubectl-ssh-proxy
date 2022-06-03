package main

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/infinite-turtles/kubectl-ssh-proxy/pkg/cmd"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-ssh-proxy", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewCmdSshProxy(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
