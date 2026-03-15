package main

import (
	"os"

	"github.com/sofq/jira-cli/cmd"
)

func main() {
	code := cmd.Execute()
	os.Exit(code)
}
