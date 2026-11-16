package main

import (
	"os"

	"github.com/Nyakooo/termihomo/internal/cli"
	"github.com/Nyakooo/termihomo/internal/tray"
)

func main() {
	if tray.HandleWorker(os.Args[1:]) {
		return
	}
	os.Exit(cli.RunWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
