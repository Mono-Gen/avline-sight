package main

import (
	"flag"
	"fmt"
	"path/filepath"
)

func printUsage(flags *flag.FlagSet) {
	visible := flag.NewFlagSet(flags.Name(), flag.ContinueOnError)
	visible.SetOutput(flags.Output())
	flags.VisitAll(func(option *flag.Flag) {
		if option.Name == "lab" || option.Name == "lab-port-offset" {
			return
		}
		visible.Var(option.Value, option.Name, option.Usage)
		visible.Lookup(option.Name).DefValue = option.DefValue
	})
	fmt.Fprintf(flags.Output(), "Usage: %s [options]\n", filepath.Base(flags.Name()))
	visible.PrintDefaults()
}
