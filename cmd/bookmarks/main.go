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
    Use:   "bookmarks",
    Short: "Extract, export, and merge browser bookmarks",
  }
  root.AddCommand(newExtractCmd())
  return root
}
