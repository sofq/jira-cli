package main

import (
	"os"

	"github.com/quanhoang/jr/cmd"
)

func main() {
	code := cmd.Execute()
	os.Exit(code)
}
