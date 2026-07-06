// Command bookmarks extracts and manages browser bookmarks.
package main

import (
  "fmt"
  "os"

  "github.com/mattn/go-isatty"
  "github.com/spf13/cobra"
)

const (
  ansiRed   = "\x1b[31m"
  ansiReset = "\x1b[0m"
)

func main() {
  if err := newRootCmd().Execute(); err != nil {
    printError(err)
    os.Exit(1)
  }
}

// printError writes err to stderr, in red when stderr is a terminal that
// supports color (skipped for NO_COLOR or non-tty output like redirects/pipes).
func printError(err error) {
  msg := fmt.Sprintf("Error: %s", err)
  if os.Getenv("NO_COLOR") == "" && isatty.IsTerminal(os.Stderr.Fd()) {
    msg = ansiRed + msg + ansiReset
  }
  fmt.Fprintln(os.Stderr, msg)
}

func newRootCmd() *cobra.Command {
  root := &cobra.Command{
    Use:           "bookmarks",
    Short:         "Extract, export, and merge browser bookmarks",
    Version:       fullVersion(),
    SilenceErrors: true,
    SilenceUsage:  true,
    Long: `bookmarks moves bookmarks between browsers that store them in completely different,
mutually incompatible ways: Chromium-family browsers (Chrome, Chromium, Brave, Edge) keep a
"Bookmarks" JSON file per profile, while Firefox and LibreWolf keep a places.sqlite database.

It works in two steps:
  1. extract - read a browser's native bookmarks into a common JSON format.
  2. import  - convert that JSON into whatever the target browser can load, and explain how to
               load it (some targets require a manual step in the browser's own UI - see
               "bookmarks import --help" for why).

Run "bookmarks <command> --help" for details and examples for a specific command.`,
  }
  root.AddCommand(newExtractCmd())
  root.AddCommand(newImportCmd())
  return root
}
