package cli

import "git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"

func echoCtrl(runner *protocol.CmdRunner, echoOn string) error {
	echoCtrl, err := protocol.NewEcho()
	if err != nil {
		return err
	}
	echoCtrl.Message = echoOn

	nmr, err := echoCtrl.EncodeEchoCtrl()
	if err != nil {
		return err
	}

	if err := runner.WriteReq(nmr); err != nil {
		return err
	}

	_, err = runner.ReadResp()
	if err != nil {
		return err
	}
	return nil
}
