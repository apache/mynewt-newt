package cli

import (
	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/newtmgr/transport"
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
