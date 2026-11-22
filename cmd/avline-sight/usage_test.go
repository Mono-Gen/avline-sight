package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestUsageHidesDeveloperLabButAcceptsItsFlags(t *testing.T) {
	flags := flag.NewFlagSet("avline-sight", flag.ContinueOnError)
	var output bytes.Buffer
	flags.SetOutput(&output)
	lab := flags.Int("lab", 0, "developer lab")
	flags.Int("lab-port-offset", 0, "developer lab ports")
	flags.String("listen", "127.0.0.1:8080", "WebUI address")
	printUsage(flags)
	if strings.Contains(output.String(), "lab") || !strings.Contains(output.String(), "-listen") || !strings.Contains(output.String(), "127.0.0.1:8080") {
		t.Fatal(output.String())
	}
	if err := flags.Parse([]string{"--lab", "4"}); err != nil || *lab != 4 {
		t.Fatalf("hidden lab flag: %d %v", *lab, err)
	}
}
