package cmd

import (
	"fmt"
	"os"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/spf13/cobra"
)

func nmUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*util.NewtError)
		fmt.Printf("ERROR: %s\n", err.Error())
		fmt.Fprintf(os.Stderr, "[DEBUG] %s", sErr.StackTrace)
	}

	if cmd != nil {
		cmd.Help()
	}

	os.Exit(1)
}
