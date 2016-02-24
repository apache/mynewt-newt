package cli

import (
	"fmt"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"github.com/spf13/cobra"
)

func statsRunCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	srr, err := protocol.NewStatsReadReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	srr.Name = args[0]

	nmr, err := srr.Encode()
	if err != nil {
		nmUsage(cmd, err)
	}

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(cmd, err)
	}

	rsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(cmd, err)
	}

	srrsp, err := protocol.DecodeStatsReadResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Return Code = %d\n", srrsp.ReturnCode)
	if srrsp.ReturnCode == 0 {
		fmt.Printf("Stats Name: %s\n", srrsp.Name)
		for k, v := range srrsp.Fields {
			fmt.Printf("  %s: %d\n", k, int(v.(float64)))
		}
	}
}

func statsCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "stat",
		Short: "Read statistics from a remote endpoint",
		Run:   statsRunCmd,
	}

	return statsCmd
}
