package cli

import (
	"fmt"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"github.com/spf13/cobra"
)

func taskStatsRunCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	srr, err := protocol.NewTaskStatsReadReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := srr.EncodeWriteRequest()
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

	tsrsp, err := protocol.DecodeTaskStatsReadResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Return Code = %d\n", tsrsp.ReturnCode)
	if tsrsp.ReturnCode == 0 {
		for k, info := range tsrsp.Tasks {
			fmt.Printf("  %s ", k)
			fmt.Printf("(prio=%d tid=%d runtime=%d cswcnt=%d stksize=%d "+
				"stkusage=%d last_checkin=%d next_checkin=%d)",
				int(info["prio"].(float64)),
				int(info["tid"].(float64)),
				int(info["runtime"].(float64)),
				int(info["cswcnt"].(float64)),
				int(info["stksiz"].(float64)),
				int(info["stkuse"].(float64)),
				int(info["last_checkin"].(float64)),
				int(info["next_checkin"].(float64)))
			fmt.Printf("\n")
		}
	}
}

func taskStatsCmd() *cobra.Command {
	taskStatsCmd := &cobra.Command{
		Use:   "taskstats",
		Short: "Read statistics from a remote endpoint",
		Run:   taskStatsRunCmd,
	}

	return taskStatsCmd
}
