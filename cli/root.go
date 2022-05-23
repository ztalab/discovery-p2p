package cli

import (
	"log"

	"github.com/DataDrake/cli-ng/v2/cmd"
)

var appVersion string = "develop"

//GlobalFlags contains the flags for commands.
type GlobalFlags struct {
	Config string `short:"c" long:"config" desc:"Specify a custom config path."`
}

// Root is the main command.
var Root *cmd.Root

func init() {
	Root = &cmd.Root{
		Name:    "discovery-p2p",
		Short:   "Discovery P2P Distributed Network",
		Version: appVersion,
		Flags:   &GlobalFlags{},
	}

	cmd.Register(&cmd.Help)
	cmd.Register(&Init)
	cmd.Register(&Up)
	cmd.Register(&cmd.Version)
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
