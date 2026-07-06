package main

import (
  "bufio"
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "time"

  "github.com/spf13/cobra"

  "bookmarks/internal/chromium"
  "bookmarks/internal/model"
  "bookmarks/internal/netscape"
)

func newImportCmd() *cobra.Command {
  var browser string
  var profile string
  var input string
  var output string
  var yes bool

  cmd := &cobra.Command{
    Use:   "import",
    Short: "Import canonical bookmarks JSON into a browser profile",
    RunE: func(cmd *cobra.Command, args []string) error {
      return runImport(browser, profile, input, output, yes)
    },
  }

  cmd.Flags().StringVar(&browser, "browser", "",
    "target browser: firefox, librewolf, chrome, chromium, brave, edge, "+
      "or a path to a custom install/profile location (required)")
  cmd.Flags().StringVar(&profile, "profile", "",
    "target profile (chromium: directory or display name, e.g. \"Default\" or \"Eduardo Sanchez\"; ignored for firefox-family targets). Defaults to \"Default\"")
  cmd.Flags().StringVar(&input, "input", "", "canonical bookmarks JSON file to import (required)")
  cmd.Flags().StringVar(&output, "output", "",
    "for firefox-family targets: where to write the generated Netscape HTML file for manual import (defaults next to --input)")
  cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt (a backup is still made for chromium-family targets)")
  cmd.MarkFlagRequired("browser")
  cmd.MarkFlagRequired("input")

  return cmd
}

func runImport(browser, profile, input, output string, yes bool) error {
  data, err := os.ReadFile(input)
  if err != nil {
    return fmt.Errorf("reading %q: %w", input, err)
  }
  var root model.Root
  if err := json.Unmarshal(data, &root); err != nil {
    return fmt.Errorf("parsing %q: %w", input, err)
  }

  if _, ok := firefoxBrowsers[browser]; ok {
    return importIntoFirefoxFamily(browser, &root, input, output)
  }
  if b, ok := chromiumBrowsers[browser]; ok {
    p := profile
    if p == "" {
      p = "Default"
    }
    path, err := chromium.ResolvePath(b, p)
    if err != nil {
      return err
    }
    return importIntoChromium(&root, path, yes)
  }
  return importIntoPath(browser, profile, &root, input, output, yes)
}

// importIntoPath handles --browser values that aren't a recognized name by
// auto-detecting the bookmark-store layout, mirroring extractFromPath.
func importIntoPath(path, profile string, root *model.Root, input, output string, yes bool) error {
  kind, err := classifyPath(path)
  if err != nil {
    return err
  }

  switch kind {
  case kindChromiumFile:
    return importIntoChromium(root, path, yes)
  case kindChromiumProfileDir:
    return importIntoChromium(root, filepath.Join(path, "Bookmarks"), yes)
  case kindChromiumRoot:
    p := profile
    if p == "" {
      p = "Default"
    }
    resolvedPath, err := chromium.ResolvePathAt(path, p)
    if err != nil {
      return err
    }
    return importIntoChromium(root, resolvedPath, yes)
  case kindFirefoxFile, kindFirefoxProfileDir, kindFirefoxRoot:
    return importIntoFirefoxFamily("firefox", root, input, output)
  }
  return fmt.Errorf("could not detect a bookmarks store under %q to import into", path)
}

// importIntoChromium overwrites the Chromium "Bookmarks" file at path with
// root's contents, after confirming with the user and backing up whatever
// was already there.
func importIntoChromium(root *model.Root, path string, yes bool) error {
  if !yes {
    ok, err := confirm(fmt.Sprintf(
      "This will REPLACE all bookmarks at:\n  %s\n\n"+
        "Make sure the browser is completely closed first - if it's running, it will overwrite\n"+
        "this change with its in-memory bookmarks the next time it saves or exits.\n"+
        "The existing file will be backed up first.", path))
    if err != nil {
      return err
    }
    if !ok {
      fmt.Println("Aborted.")
      return nil
    }
  }

  if fileExists(path) {
    backupPath := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
    if err := copyFileTo(path, backupPath); err != nil {
      return fmt.Errorf("backing up existing bookmarks: %w", err)
    }
    fmt.Printf("Backed up existing bookmarks to %s\n", backupPath)
  }

  warnings, err := chromium.WriteFile(root, path)
  for _, w := range warnings {
    fmt.Printf("warning: %s\n", w)
  }
  if err != nil {
    return err
  }

  fmt.Printf("Wrote %s\n", path)
  fmt.Println("Fully quit and relaunch the browser to see the imported bookmarks.")
  return nil
}

// importIntoFirefoxFamily generates a Netscape Bookmark HTML file, since
// writing places.sqlite directly isn't safe (see internal/netscape).
func importIntoFirefoxFamily(name string, root *model.Root, input, output string) error {
  if output == "" {
    output = strings.TrimSuffix(input, filepath.Ext(input)) + ".html"
  }
  f, err := os.Create(output)
  if err != nil {
    return fmt.Errorf("creating %q: %w", output, err)
  }
  defer f.Close()
  if err := netscape.Write(root, f); err != nil {
    return err
  }

  fmt.Printf("Wrote a Netscape bookmarks file to %s\n", output)
  fmt.Println()
  fmt.Printf("%s stores bookmarks in a live SQLite database with internal bookkeeping (custom\n", capitalize(name))
  fmt.Println("SQL functions, generated columns, shared history data) this tool can't safely replicate,")
  fmt.Println("so it can't be written directly. To finish the import:")
  fmt.Println("  1. Open the browser and go to the Bookmarks Library (Ctrl+Shift+O / Cmd+Shift+O).")
  fmt.Println("  2. Library > Import and Backup > Import Bookmarks from HTML File...")
  fmt.Printf("  3. Select %s\n", output)
  return nil
}

func confirm(prompt string) (bool, error) {
  fmt.Println(prompt)
  fmt.Print("Type \"yes\" to continue: ")
  line, err := bufio.NewReader(os.Stdin).ReadString('\n')
  if err != nil && line == "" {
    return false, err
  }
  return strings.TrimSpace(strings.ToLower(line)) == "yes", nil
}

func copyFileTo(src, dst string) error {
  data, err := os.ReadFile(src)
  if err != nil {
    return err
  }
  return os.WriteFile(dst, data, 0o644)
}

func capitalize(s string) string {
  if s == "" {
    return s
  }
  return strings.ToUpper(s[:1]) + s[1:]
}
