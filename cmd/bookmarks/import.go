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
  var dryRun bool

  cmd := &cobra.Command{
    Use:   "import",
    Short: "Import canonical bookmarks JSON into a browser profile",
    Long: `Converts the canonical JSON produced by "bookmarks extract" into whatever a target
browser can actually load, and explains how to load it.

By default this generates a Netscape-format HTML file and prints instructions for the browser's
own bookmark-import feature - a safe, non-destructive merge that adds a new folder alongside
whatever bookmarks are already there.

Pass --replace to instead overwrite the target's ENTIRE bookmark tree:
  - Chromium-family targets (chrome/chromium/brave/edge): writes the Bookmarks file directly
    (the existing file is backed up first, and you'll be asked to confirm unless --yes is set).
  - Firefox-family targets (firefox/librewolf): Firefox has no safe way to write its bookmark
    database directly (it relies on custom SQLite functions only Firefox's own process
    registers), so --replace instead generates a full-tree backup file for Firefox's own
    Restore feature - still a manual step, but one that preserves toolbar/menu/other placement,
    unlike the default HTML merge.

--browser accepts a recognized name or a path, same as "bookmarks extract" - see
"bookmarks extract --help" for details.

Use --dry-run to preview the target, any conversion warnings, and a bookmark summary without
writing, backing up, or prompting for anything.`,
    Example: `  bookmarks import --browser chrome --profile test --input bookmarks.json
  bookmarks import --browser firefox --input bookmarks.json --replace
  bookmarks import --browser edge --input bookmarks.json --replace --dry-run`,
    RunE: func(cmd *cobra.Command, args []string) error {
      return runImport(browser, profile, input, output, yes, replace, dryRun)
    },
  }

  cmd.Flags().StringVar(&browser, "browser", "",
    "target browser: firefox, librewolf, chrome, chromium, brave, edge, "+
      "or a path to a custom install/profile location (required)")
  cmd.Flags().StringVar(&profile, "profile", "",
    "target profile: directory name or display name (ignored for firefox-family targets). Defaults to \"Default\"")
  cmd.Flags().StringVar(&input, "input", "", "canonical bookmarks JSON file to import (required)")
  cmd.Flags().StringVarP(&output, "output", "o", "",
    "where to write the generated file for manual import/restore (defaults next to --input)")
  cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt when --replace targets a chromium-family browser")
  cmd.Flags().BoolVar(&replace, "replace", false,
    "replace ALL of the target's bookmarks directly instead of generating a file to merge in manually (see below)")
  cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would happen (target, warnings, summary), without writing, backing up, or prompting")
  cmd.MarkFlagRequired("browser")
  cmd.MarkFlagRequired("input")

  return cmd
}

func runImport(browser, profile, input, output string, yes, replace, dryRun bool) error {
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
      return importIntoFirefoxReplace(browser, &root, input, output, dryRun)
    }
    return importIntoFirefoxMerge(browser, &root, input, output, dryRun)
  }
  if b, ok := chromiumBrowsers[browser]; ok {
    if !replace {
      return importIntoChromiumMerge(browser, &root, input, output, dryRun)
    }
    p := profile
    if p == "" {
      p = "Default"
    }
    path, err := chromium.ResolvePath(b, p)
    if err != nil {
      return err
    }
    return importIntoChromiumReplace(&root, path, input, yes, dryRun)
  }
  return importIntoPath(browser, profile, &root, input, output, yes, replace, dryRun)
}

// importIntoPath handles --browser values that aren't a recognized name by
// auto-detecting the bookmark-store layout, mirroring extractFromPath.
func importIntoPath(path, profile string, root *model.Root, input, output string, yes, replace, dryRun bool) error {
  kind, err := classifyPath(path)
  if err != nil {
    return err
  }

  switch kind {
  case kindChromiumFile:
    if !replace {
      return importIntoChromiumMerge("chromium", root, input, output, dryRun)
    }
    return importIntoChromiumReplace(root, path, input, yes, dryRun)
  case kindChromiumProfileDir:
    if !replace {
      return importIntoChromiumMerge("chromium", root, input, output, dryRun)
    }
    return importIntoChromiumReplace(root, filepath.Join(path, "Bookmarks"), input, yes, dryRun)
  case kindChromiumRoot:
    if !replace {
      return importIntoChromiumMerge("chromium", root, input, output, dryRun)
    }
    p := profile
    if p == "" {
      p = "Default"
    }
    resolvedPath, err := chromium.ResolvePathAt(path, p)
    if err != nil {
      return err
    }
    return importIntoChromiumReplace(root, resolvedPath, input, yes, dryRun)
  case kindFirefoxFile, kindFirefoxProfileDir, kindFirefoxRoot:
    if replace {
      return importIntoFirefoxReplace("firefox", root, input, output, dryRun)
    }
    return importIntoFirefoxMerge("firefox", root, input, output, dryRun)
  }
  return fmt.Errorf("could not detect a bookmarks store under %q to import into", path)
}

func printImportDryRunHeader(target, input string, root *model.Root) {
  dryRunHeader()
  fmt.Printf("Target:        %s\n", target)
  fmt.Printf("Reading from:  %s\n", input)
  fmt.Println()
  printBookmarkSummary(root)
  fmt.Println()
}

// importIntoChromiumReplace overwrites the Chromium "Bookmarks" file at
// path with root's contents, after confirming with the user and backing up
// whatever was already there.
func importIntoChromiumReplace(root *model.Root, path, input string, yes, dryRun bool) error {
  if dryRun {
    printImportDryRunHeader(path, input, root)
    _, warnings := chromium.BuildFile(root)
    for _, w := range warnings {
      fmt.Printf("warning: %s\n", w)
    }
    fmt.Println("Action: REPLACE - would overwrite the file above directly.")
    if fileExists(path) {
      fmt.Println("A backup of the existing file would be made first.")
    } else {
      fmt.Println("No existing file at that path - nothing to back up.")
    }
    return nil
  }

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
func importIntoChromiumMerge(name string, root *model.Root, input, output string, dryRun bool) error {
  outPath := resolveHTMLOutputPath(input, output)
  if dryRun {
    printImportDryRunHeader(fmt.Sprintf("%s (via Import bookmarks - merge)", capitalize(name)), input, root)
    fmt.Printf("Action: MERGE - would write a Netscape bookmarks file to:\n  %s\n", outPath)
    fmt.Println("No existing bookmarks would be touched by this tool; merging happens when you use")
    fmt.Println("the browser's own \"Import bookmarks\" feature on that file.")
    return nil
  }

  path, err := writeHTMLFile(root, outPath)
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
func importIntoFirefoxReplace(name string, root *model.Root, input, output string, dryRun bool) error {
  outPath := output
  if outPath == "" {
    outPath = strings.TrimSuffix(input, filepath.Ext(input)) + ".json"
  }

  if dryRun {
    printImportDryRunHeader(fmt.Sprintf("%s (via Restore - replace)", capitalize(name)), input, root)
    _, warnings := firefox.BuildBackup(root)
    for _, w := range warnings {
      fmt.Printf("warning: %s\n", w)
    }
    fmt.Printf("Action: REPLACE - would write a bookmarks backup file to:\n  %s\n", outPath)
    fmt.Println("Restoring that file in the browser replaces its entire bookmark tree; nothing is")
    fmt.Println("touched until you do that manually.")
    return nil
  }

  f, err := os.Create(outPath)
  if err != nil {
    return fmt.Errorf("creating %q: %w", outPath, err)
  }
  defer f.Close()
  warnings, err := firefox.WriteBackup(root, f)
  for _, w := range warnings {
    fmt.Printf("warning: %s\n", w)
  }
  if err != nil {
    return err
  }

  fmt.Printf("Wrote a bookmarks backup file to %s\n", outPath)
  fmt.Println()
  fmt.Printf("%s stores bookmarks in a live SQLite database with internal bookkeeping (custom\n", capitalize(name))
  fmt.Println("SQL functions, generated columns, shared history data) this tool can't safely replicate,")
  fmt.Println("so it can't be written directly. To finish the import:")
  fmt.Println("  1. Open the browser and go to the Bookmarks Library (Ctrl+Shift+O / Cmd+Shift+O).")
  fmt.Println("  2. Library > Import and Backup > Restore > Choose File...")
  fmt.Printf("  3. Select %s\n", outPath)
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
func importIntoFirefoxMerge(name string, root *model.Root, input, output string, dryRun bool) error {
  outPath := resolveHTMLOutputPath(input, output)
  if dryRun {
    printImportDryRunHeader(fmt.Sprintf("%s (via Import Bookmarks from HTML File - merge)", capitalize(name)), input, root)
    fmt.Printf("Action: MERGE - would write a Netscape bookmarks file to:\n  %s\n", outPath)
    fmt.Println("No existing bookmarks would be touched by this tool; merging happens when you use")
    fmt.Println("the browser's own HTML importer on that file (everything lands under Bookmarks Menu).")
    return nil
  }

  path, err := writeHTMLFile(root, outPath)
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

func resolveHTMLOutputPath(input, output string) string {
  if output != "" {
    return output
  }
  return strings.TrimSuffix(input, filepath.Ext(input)) + ".html"
}

func writeHTMLFile(root *model.Root, outPath string) (string, error) {
  f, err := os.Create(outPath)
  if err != nil {
    return "", fmt.Errorf("creating %q: %w", outPath, err)
  }
  defer f.Close()
  if err := netscape.Write(root, f); err != nil {
    return "", err
  }
  fmt.Printf("Wrote a Netscape bookmarks file to %s\n", outPath)
  return outPath, nil
}

func confirm(prompt string) (bool, error) {
  fmt.Println(prompt)
  fmt.Print("Type \"yes\" to continue: ")
  line, err := bufio.NewReader(os.Stdin).ReadString('\n')
  if err != nil && line == "" {
    return false, fmt.Errorf("reading confirmation input: %w", err)
  }
  return strings.TrimSpace(strings.ToLower(line)) == "yes", nil
}

func copyFileTo(src, dst string) error {
  data, err := os.ReadFile(src)
  if err != nil {
    return fmt.Errorf("reading %q: %w", src, err)
  }
  if err := os.WriteFile(dst, data, 0o644); err != nil {
    return fmt.Errorf("writing %q: %w", dst, err)
  }
  return nil
}

func capitalize(s string) string {
  if s == "" {
    return s
  }
  return strings.ToUpper(s[:1]) + s[1:]
}
