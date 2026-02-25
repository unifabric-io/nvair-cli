package main

import (
	"os"

	"github.com/unifabric-io/nvair-cli/pkg/commands"
)

func main() {
	root := commands.NewRootCommand()
	code := root.Run(os.Args[1:])
	os.Exit(code)
}
