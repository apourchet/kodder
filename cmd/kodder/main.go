package main

import (
	"log"
	"os"

	"github.com/apourchet/kodder/lib/cli"
)

func main() {
	application := cli.NewClientApplication()
	go application.HandleSignals()

	lastArg := os.Args[len(os.Args)-1]
	makisuArgs := os.Args[2 : len(os.Args)-1]

	var err error
	switch os.Args[1] {
	case "build":
		err = application.Build(lastArg, makisuArgs)
	case "abort":
		err = application.Abort()
	case "ready":
		err = application.Ready()
	}
	if err != nil {
		log.Fatalf("FATAL: %s", err)
	}
}
