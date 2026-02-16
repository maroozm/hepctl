package main

import (
	"context"
	"fmt"
	"os"

	"hepctl/internal/install"
	"hepctl/internal/ui"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]

	if len(args) == 0 {
		if err := runInteractive(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(args) == 2 && args[0] == "install" && args[1] == "root" {
		installer := install.NewRootInstaller(
			install.DefaultRunner{},
			ui.NewPrompter(os.Stdin, os.Stdout),
			os.Stdout,
		)
		if err := installer.Install(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	printUsage()
	os.Exit(2)
}

func runInteractive(_ context.Context) error {
	return ui.RunDashboard()
}

func printUsage() {
	fmt.Println("hepctl - HEP package installer")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hepctl                  # interactive mode")
	fmt.Println("  hepctl install root     # install ROOT")
}
