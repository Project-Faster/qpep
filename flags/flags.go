package flags

import (
	"flag"
	"os"
)

type GlobalFlags struct {
	Service string
	Client  bool
	Verbose bool
}

var (
	Globals GlobalFlags
)

func init() {
	ParseFlags(os.Args)
}

func ParseFlags(args []string) {
	serviceFlag := flag.String("service", "", "Flag to indicate the service action to be done")
	clientFlag := flag.Bool("client", false, "indicates if the operation is in client or server mode")
	verbose := flag.Bool("verbose", false, "Outputs data about diverted connections for debug")

	flag.CommandLine.Parse(args[1:])
	if !flag.Parsed() {
		flag.Usage()
		os.Exit(1)
	}

	Globals = GlobalFlags{
		Service: *serviceFlag,
		Client:  *clientFlag,
		Verbose: *verbose,
	}
}
