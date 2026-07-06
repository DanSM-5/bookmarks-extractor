// Command bookmarks extracts and manages browser bookmarks.
package main

import (
  "fmt"
  "os"

  "github.com/spf13/cobra"
)

func main() {
  if err := newRootCmd().Execute(); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
}

func newRootCmd() *cobra.Command {
  root := &cobra.Command{
    Use:     "bookmarks",
    Short:   "Extract, export, and merge browser bookmarks",
    Version: fullVersion(),
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
