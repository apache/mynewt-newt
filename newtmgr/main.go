package main

import(
	"mynewt.apache.org/newt/newtmgr/protocol"
 	"github.com/gopherjs/gopherjs/js"
	)

func main() {
	js.Global.Set("reset", map[string]interface{}{
		"New": New,
	})
}



func New() *js.Object {
	return js.MakeWrapper(&protocol.Reset{})
}
