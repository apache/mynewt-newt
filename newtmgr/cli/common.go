package cli

import (
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/config"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/transport"
)

func getTargetCmdRunner() (*protocol.CmdRunner, error) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		return nil, err
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		return nil, err
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		return nil, err
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		return nil, err
	}
	return runner, nil
}
