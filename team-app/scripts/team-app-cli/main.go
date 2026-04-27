package main

import (
	"fmt"
	"os"
)

// team-app-cli is the operator CLI for the team-app standalone server.
// Story 8.9 lands the `org bootstrap` subcommand. Until then, this binary
// is a placeholder so the directory survives git and the binary path is
// reserved.
func main() {
	fmt.Fprintln(os.Stderr, "team-app-cli: not implemented yet (Story 8.9 ships `org bootstrap`)")
	os.Exit(2)
}
