package cli

import (
	"fmt"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/spf13/cobra"
)

func configRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, util.NewNewtError("Need variable name"))
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	config, err := protocol.NewConfig()
	if err != nil {
		nmUsage(cmd, err)
	}

	config.Name = args[0]
	if len(args) > 1 {
		config.Value = args[1]
	}
	nmr, err := config.EncodeRequest()
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

	configRsp, err := protocol.DecodeConfigResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if configRsp.Value != "" {
		fmt.Printf("Value: %s\n", configRsp.Value)
	}
}

func configCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "config",
		Short: "Read or write config value on target",
		Run:   configRunCmd,
	}

	return statsCmd
}
