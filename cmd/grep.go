package cmd

import (
	"fmt"

	"github.com/major0/kbchat/config"
)

// RunGrep executes the grep subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunGrep(_ []string, _ *config.Config) error {
	fmt.Println("grep: not implemented")
	return nil
}
