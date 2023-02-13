/*
 * Package flags implements the logic necessary to control the commands to the service
 * and its options, with the possibility of overriding explicitly the options set in
 * the configuration files.
 */
package flags

import (
	"github.com/jessevdk/go-flags"
	"os"
	"strings"
)

// GlobalFlags struct models the options that can be specified on the command line to
// control the behavior of the service
type GlobalFlags struct {
	// Service (flag:service) is a string that instruct the service to execute a specific command (eg. start/stop)
	Service string `long:"service" description:"Service command"`
	// Client (flag:client / c) if present indicates that the command is to be execute as a client (instead of server)
	Client bool `long:"client" short:"c" description:"Client mode instead of Server mode"`
	// Verbose (flag:verbose / v) if present indicates to output more log messages for debug purposes
	Verbose bool `long:"verbose" short:"v" description:"Outputs more log messages"`

	// ConfigOverrideCallback is the function callback called by the go-flags package to allow the parsing
	// of multiple definitions of the same D flag
	// the reason this is exported is to allow the go-flags package to discover it
	ConfigOverrideCallback func(string) `short:"D" description:"Allow to override configuration values with the format name=value"`
	// ConfigOverrides is the map in which the ConfigOverrideCallback sets the parsed "name=value" pairs specified
	// on the commandline
	ConfigOverrides map[string]string
}

var (
	// Globals variable contains the specified options on the cli
	Globals GlobalFlags
)

// ParseFlags method executes the logic that parses correctly the flags specified on the cli
func ParseFlags(args []string) {
	Globals = GlobalFlags{}
	Globals.ConfigOverrides = make(map[string]string)
	Globals.ConfigOverrideCallback = func(s string) {
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
