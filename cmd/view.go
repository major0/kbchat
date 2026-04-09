package cmd

import (
	"fmt"

	"github.com/major0/kbchat/config"
)

// RunView executes the view subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunView(args []string, cfg *config.Config) error {
	fmt.Println("view: not implemented")
	return nil
}
