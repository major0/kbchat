package cmd

import (
	"fmt"

	"github.com/major0/kbchat/config"
)

// RunSearch executes the search subcommand.
// args contains the remaining arguments after subcommand dispatch.
func RunSearch(args []string, cfg *config.Config) error {
	fmt.Println("search: not implemented")
	return nil
}
