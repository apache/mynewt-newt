package cmd

import (
	"fmt"
	"strings"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/runtimeinc/xolo/cli"
	"github.com/spf13/cobra"
)

func connProfileAddCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]
	cp, err := cli.NewConnProfile(name)
	if err != nil {
		nmUsage(cmd, err)
	}

	for _, vdef := range args[1:] {
		s := strings.Split(vdef, "=")
		switch s[0] {
		case "name":
			cp.MyName = s[1]
		case "type":
			cp.MyType = s[1]
		case "connstring":
			cp.MyConnString = s[1]
		default:
			nmUsage(cmd, util.NewNewtError("Unknown variable "+s[0]))
		}
	}

	if err := cpm.AddConnProfile(cp); err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Connection profile %s successfully added\n", name)
}

func connProfileShowCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	cpList, err := cpm.GetConnProfileList()
	if err != nil {
		nmUsage(cmd, err)
	}

	found := false
	for _, cp := range cpList {
		// Print out the connection profile, if name is "" or name
		// matches cp.Name
		if name != "" && cp.Name() != name {
			continue
		}

		if !found {
			found = true
			fmt.Printf("Connection profiles: \n")
		}
		fmt.Printf("  %s: type=%s, connstring='%s'\n", cp.MyName, cp.MyType,
			cp.MyConnString)
	}

	if !found {
		if name == "" {
			fmt.Printf("No connection profiles found!\n")
		} else {
			fmt.Printf("No connection profiles found matching %s\n", name)
		}
	}
}

func connProfileDelCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]

	if err := cpm.DeleteConnProfile(name); err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Connection profile %s successfully deleted.\n", name)
}

func connProfileCmd() *cobra.Command {
	cpCmd := &cobra.Command{
		Use:   "conn",
		Short: "Manage newtmgr connection profiles",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a newtmgr connection profile",
		Run:   connProfileAddCmd,
	}
	cpCmd.AddCommand(addCmd)

	deleCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a newtmgr connection profile",
		Run:   connProfileDelCmd,
	}
	cpCmd.AddCommand(deleCmd)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show newtmgr connection profiles",
		Run:   connProfileShowCmd,
	}
	cpCmd.AddCommand(showCmd)

	return cpCmd
}
