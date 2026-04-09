package cmd

import (
	"fmt"

	"github.com/major0/kbchat/config"
)

// RunList executes the list subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunList(_ []string, _ *config.Config) error {
	fmt.Println("list: not implemented")
	return nil
}
