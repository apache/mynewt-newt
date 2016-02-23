package cmd

import (
	"log"
	"os"

	"github.com/hashicorp/logutils"
	"github.com/spf13/cobra"
)

var ConnProfileName string

var LogLevel string = "WARN"

func SetupLog() {
	filter := &logutils.LevelFilter{
		Levels: []logutils.LogLevel{"DEBUG", "VERBOSE", "INFO",
			"WARN", "ERROR"},
		MinLevel: logutils.LogLevel(LogLevel),
		Writer:   os.Stderr,
	}

	log.SetOutput(filter)
}

func Parse() *cobra.Command {
	nmCmd := &cobra.Command{
		Use:   "newtmgr",
		Short: "Newtmgr helps you manage remote instances of the Mynewt OS.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	nmCmd.PersistentFlags().StringVarP(&ConnProfileName, "conn", "c", "",
		"connection profile to use.")

	nmCmd.PersistentFlags().StringVarP(&LogLevel, "loglevel", "l", "",
		"log level to use (default WARN.)")

	nmCmd.AddCommand(connProfileCmd())
	nmCmd.AddCommand(echoCmd())
	nmCmd.AddCommand(imageCmd())
	nmCmd.AddCommand(statsCmd())
	nmCmd.AddCommand(taskStatsCmd())
	nmCmd.AddCommand(mempoolStatsCmd())
	nmCmd.AddCommand(configCmd())
	nmCmd.AddCommand(logsCmd())

	return nmCmd
}
