package main

import "github.com/brianmcarey/podvirt/cmd"

var version = "dev"

func main() {
	cmd.Execute(version)
}
