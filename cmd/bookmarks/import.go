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
  "bookmarks/internal/firefox"
  "bookmarks/internal/model"
  "bookmarks/internal/netscape"
)

func newImportCmd() *cobra.Command {
  var browser string
  var profile string
  var input string
  var output string
  var yes bool
  var replace bool

  cmd := &cobra.Command{
    Use:   "import",
    Short: "Import canonical bookmarks JSON into a browser profile",
    RunE: func(cmd *cobra.Command, args []string) error {
      return runImport(browser, profile, input, output, yes, replace)
    },
  }

  cmd.Flags().StringVar(&browser, "browser", "",
    "target browser: firefox, librewolf, chrome, chromium, brave, edge, "+
      "or a path to a custom install/profile location (required)")
  cmd.Flags().StringVar(&profile, "profile", "",
    "target profile (chromium: directory or display name, e.g. \"Default\" or \"Eduardo Sanchez\"; ignored for firefox-family targets). Defaults to \"Default\"")
  cmd.Flags().StringVar(&input, "input", "", "canonical bookmarks JSON file to import (required)")
  cmd.Flags().StringVarP(&output, "output", "o", "",
    "where to write the generated file for manual import/restore (defaults next to --input)")
  cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt when --replace targets a chromium-family browser")
  cmd.Flags().BoolVar(&replace, "replace", false,
    "replace ALL of the target's bookmarks instead of generating a file to merge in manually. "+
      "For chromium-family targets this writes the Bookmarks file directly (with a backup and confirmation). "+
      "Firefox has no safe direct-write path either way, so this only changes which file is generated: "+
      "a full-tree backup for its Restore feature (replaces everything) instead of an HTML file for its "+
      "merge-style HTML import.")
  cmd.MarkFlagRequired("browser")
  cmd.MarkFlagRequired("input")

  return cmd
}

func runImport(browser, profile, input, output string, yes, replace bool) error {
  data, err := os.ReadFile(input)
  if err != nil {
    return fmt.Errorf("reading %q: %w", input, err)
  }
  var root model.Root
  if err := json.Unmarshal(data, &root); err != nil {
    return fmt.Errorf("parsing %q: %w", input, err)
  }

  if _, ok := firefoxBrowsers[browser]; ok {
    if replace {
      return importIntoFirefoxReplace(browser, &root, input, output)
    }
    return importIntoFirefoxMerge(browser, &root, input, output)
  }
  if b, ok := chromiumBrowsers[browser]; ok {
    if !replace {
      return importIntoChromiumMerge(browser, &root, input, output)
    }
    p := profile
    if p == "" {
      p = "Default"
    }
    path, err := chromium.ResolvePath(b, p)
    if err != nil {
      return err
    }
    return importIntoChromiumReplace(&root, path, yes)
  }
  return importIntoPath(browser, profile, &root, input, output, yes, replace)
}

// importIntoPath handles --browser values that aren't a recognized name by
// auto-detecting the bookmark-store layout, mirroring extractFromPath.
func importIntoPath(path, profile string, root *model.Root, input, output string, yes, replace bool) error {
  kind, err := classifyPath(path)
  if err != nil {
    return err
  }

  switch kind {
  case kindChromiumFile:
    if !replace {
      return importIntoChromiumMerge("chromium", root, input, output)
    }
    return importIntoChromiumReplace(root, path, yes)
  case kindChromiumProfileDir:
    if !replace {
      return importIntoChromiumMerge("chromium", root, input, output)
    }
    return importIntoChromiumReplace(root, filepath.Join(path, "Bookmarks"), yes)
  case kindChromiumRoot:
    if !replace {
      return importIntoChromiumMerge("chromium", root, input, output)
    }
    p := profile
    if p == "" {
      p = "Default"
    }
    resolvedPath, err := chromium.ResolvePathAt(path, p)
    if err != nil {
      return err
    }
    return importIntoChromiumReplace(root, resolvedPath, yes)
  case kindFirefoxFile, kindFirefoxProfileDir, kindFirefoxRoot:
    if replace {
      return importIntoFirefoxReplace("firefox", root, input, output)
    }
    return importIntoFirefoxMerge("firefox", root, input, output)
  }
  return fmt.Errorf("could not detect a bookmarks store under %q to import into", path)
}

// importIntoChromiumReplace overwrites the Chromium "Bookmarks" file at
// path with root's contents, after confirming with the user and backing up
// whatever was already there.
func importIntoChromiumReplace(root *model.Root, path string, yes bool) error {
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

// importIntoChromiumMerge generates a Netscape HTML file and points the
// user at the browser's own "Import bookmarks" feature, which adds the
// imported bookmarks into a new folder without touching what's already
// there.
func importIntoChromiumMerge(name string, root *model.Root, input, output string) error {
  path, err := writeHTMLFile(root, input, output)
  if err != nil {
    return err
  }
  fmt.Println()
  fmt.Println("This won't touch the browser directly. To merge these bookmarks in without disturbing")
  fmt.Println("what's already there:")
  fmt.Printf("  1. Open %s's bookmark manager (e.g. chrome://bookmarks or edge://favorites).\n", capitalize(name))
  fmt.Println("  2. Use its \"Import bookmarks\" option (usually under the ⋮ / … menu) and select:")
  fmt.Printf("     %s\n", path)
  fmt.Println("  (Or open that file directly in the browser and drag individual links onto your bookmarks bar.)")
  fmt.Println()
  fmt.Println("Pass --replace to instead overwrite all of the browser's bookmarks directly (with a backup and confirmation).")
  return nil
}

// importIntoFirefoxReplace generates a Firefox-native bookmarks backup JSON
// file, since writing places.sqlite directly isn't safe (see
// internal/firefox/backup.go). Restoring from this format replaces the
// browser's entire bookmark tree, but - unlike the merge-style HTML import
// - preserves toolbar/menu/other placement.
func importIntoFirefoxReplace(name string, root *model.Root, input, output string) error {
  if output == "" {
    output = strings.TrimSuffix(input, filepath.Ext(input)) + ".json"
  }
  f, err := os.Create(output)
  if err != nil {
    return fmt.Errorf("creating %q: %w", output, err)
  }
  defer f.Close()
  warnings, err := firefox.WriteBackup(root, f)
  for _, w := range warnings {
    fmt.Printf("warning: %s\n", w)
  }
  if err != nil {
    return err
  }

  fmt.Printf("Wrote a bookmarks backup file to %s\n", output)
  fmt.Println()
  fmt.Printf("%s stores bookmarks in a live SQLite database with internal bookkeeping (custom\n", capitalize(name))
  fmt.Println("SQL functions, generated columns, shared history data) this tool can't safely replicate,")
  fmt.Println("so it can't be written directly. To finish the import:")
  fmt.Println("  1. Open the browser and go to the Bookmarks Library (Ctrl+Shift+O / Cmd+Shift+O).")
  fmt.Println("  2. Library > Import and Backup > Restore > Choose File...")
  fmt.Printf("  3. Select %s\n", output)
  fmt.Println()
  fmt.Println("Note: this REPLACES the browser's entire bookmark tree. Back up existing bookmarks first")
  fmt.Println("if you want to keep them (Library > Import and Backup > Backup...).")
  return nil
}

// importIntoFirefoxMerge generates a Netscape HTML file and points the user
// at Firefox's HTML importer, which merges into the existing Bookmarks
// Menu instead of replacing anything - see the docstring on
// internal/firefox/backup.go for why this doesn't preserve toolbar
// placement the way importIntoFirefoxReplace does.
func importIntoFirefoxMerge(name string, root *model.Root, input, output string) error {
  path, err := writeHTMLFile(root, input, output)
  if err != nil {
    return err
  }
  fmt.Println()
  fmt.Println("This won't touch the browser directly. To merge these bookmarks in without disturbing")
  fmt.Println("what's already there:")
  fmt.Println("  1. Open the Bookmarks Library (Ctrl+Shift+O / Cmd+Shift+O).")
  fmt.Println("  2. Import and Backup > Import Bookmarks from HTML File...")
  fmt.Printf("  3. Select %s\n", path)
  fmt.Println("  (Or open that file directly in the browser and drag individual links onto your bookmarks bar.)")
  fmt.Println()
  fmt.Println("Note: this always lands everything under Bookmarks Menu, even bookmarks that were")
  fmt.Println("originally on a toolbar - Firefox's HTML importer doesn't redistribute across roots.")
  fmt.Printf("Pass --replace to instead generate a full-tree backup for %s's Restore feature, which\n", capitalize(name))
  fmt.Println("does preserve toolbar/other placement but replaces everything.")
  return nil
}

func writeHTMLFile(root *model.Root, input, output string) (string, error) {
  if output == "" {
    output = strings.TrimSuffix(input, filepath.Ext(input)) + ".html"
  }
  f, err := os.Create(output)
  if err != nil {
    return "", fmt.Errorf("creating %q: %w", output, err)
  }
  defer f.Close()
  if err := netscape.Write(root, f); err != nil {
    return "", err
  }
  fmt.Printf("Wrote a Netscape bookmarks file to %s\n", output)
  return output, nil
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
