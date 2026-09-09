package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "agent-context" {
		fmt.Println(`{"commands":[{"name":"live"}]}`)
		return
	}
	if len(args) > 0 && args[0] == "live" {
		fmt.Println("Live command\n\nUsage:\n  matrix-pass live [flags]\n\nExamples:\n  matrix-pass live --limit 10\n\nFlags:\n  --limit int   max results")
		return
	}
	fmt.Println("Usage:\n  matrix-pass [command]\n\nAvailable Commands:\n  live\n\nFlags:\n  --help   help for matrix-pass")
}
