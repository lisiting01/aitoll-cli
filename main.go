package main

import (
	"github.com/lisiting01/aitoll-cli/cmd"
)

// version is set at build time via -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
