package main

import (
	"log"
	"os"

	"github.com/apourchet/commander"
	"github.com/apourchet/kodder/lib/cli"
)

func main() {
	application := cli.NewClientApplication()
	cmd := commander.New()
	if err := cmd.RunCLI(application, os.Args[1:]); err != nil {
		log.Fatalf("FATAL: %s", err)
	}
}
