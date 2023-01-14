package flags

import (
	"github.com/jessevdk/go-flags"
	"os"
	"strings"
)

type GlobalFlags struct {
	Service string `long:"service" description:"Service command"`
	Client  bool   `long:"client" short:"c" description:"Client mode instead of Server mode"`
	Verbose bool   `long:"verbose" short:"s" description:"Outputs more log messages"`

	ConfigOverrideCallback func(string) `short:"D" description:"Allow to override configuration values with the format name=value"`
	ConfigOverrides        map[string]string
}

var (
	Globals GlobalFlags
)

func init() {
	parseFlags(os.Args)
}

func parseFlags(args []string) {
	Globals.ConfigOverrideCallback = func(s string) {
		if Globals.ConfigOverrides == nil {
			Globals.ConfigOverrides = make(map[string]string)
		}
		index := strings.Index(s, "=")
		if index == -1 {
			return
		}
		Globals.ConfigOverrides[strings.ToLower(s[:index])] = s[index+1:]
	}

	cliparser := flags.NewParser(&Globals, flags.Default)
	if _, err := cliparser.ParseArgs(args); err != nil {
		cliparser.WriteHelp(os.Stderr)
		os.Exit(1)
	}
}
