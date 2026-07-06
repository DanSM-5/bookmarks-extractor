package main

import (
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"

  "github.com/spf13/cobra"

  "bookmarks/internal/chromium"
  "bookmarks/internal/firefox"
  "bookmarks/internal/model"
)

// chromiumBrowsers maps recognized --browser names to a Chromium-family
// browser. Multiple names may map to the same browser (aliases).
var chromiumBrowsers = map[string]chromium.Browser{
  "chrome":        chromium.Chrome,
  "google-chrome": chromium.Chrome,
  "chromium":      chromium.Chromium,
  "brave":         chromium.Brave,
  "edge":          chromium.Edge,
}

// firefoxBrowsers maps recognized --browser names to a Gecko-family
// product.
var firefoxBrowsers = map[string]firefox.Product{
  "firefox":   firefox.Firefox,
  "librewolf": firefox.LibreWolf,
}

func newExtractCmd() *cobra.Command {
  var browser string
  var profile string
  var output string
  var listProfiles bool
  var dryRun bool

  cmd := &cobra.Command{
    Use:   "extract",
    Short: "Extract bookmarks from a browser profile into the canonical JSON format",
    Long: `Reads a browser's native bookmark storage - a Chromium "Bookmarks" JSON file, or a
Firefox/LibreWolf places.sqlite database - and converts it into a common JSON format that
"bookmarks import" can later read.

--browser accepts either a recognized name (firefox, librewolf, chrome, chromium, brave, edge)
or a path: a Bookmarks file, a places.sqlite file, a profile directory, or a user-data/profiles
root containing multiple profiles. The bookmark-store layout is auto-detected from the path.

--profile accepts a directory name (e.g. "Default", "Profile 1"), a display name shown in the
browser's own profile picker (e.g. "Eduardo Sanchez"), or a Firefox profiles.ini profile name.
Run with --list-profiles first if you're not sure what's available.`,
    Example: `  bookmarks extract --browser chrome -o bookmarks.json
  bookmarks extract --browser firefox --profile default-release --dry-run
  bookmarks extract --browser chrome --list-profiles
  bookmarks extract --browser /path/to/custom/profile -o out.json`,
    RunE: func(cmd *cobra.Command, args []string) error {
      if listProfiles {
        return runListProfiles(browser)
      }
      return runExtract(browser, profile, output, dryRun)
    },
  }

  cmd.Flags().StringVar(&browser, "browser", "",
    "browser to extract from: firefox, librewolf, chrome, chromium, brave, edge, "+
      "or a path to a custom install/profile location (required)")
  cmd.Flags().StringVar(&profile, "profile", "", "profile: directory name, display name, or profiles.ini name (see --list-profiles). Defaults to the browser's default profile")
  cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (defaults to stdout)")
  cmd.Flags().BoolVar(&listProfiles, "list-profiles", false, "list available profiles for --browser and exit")
  cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be read and written, without writing anything")
  cmd.MarkFlagRequired("browser")

  return cmd
}

func runListProfiles(browser string) error {
  if product, ok := firefoxBrowsers[browser]; ok {
    profiles, err := firefox.ListProfiles(product)
    if err != nil {
      return err
    }
    printFirefoxProfiles(profiles)
    return nil
  }
  if b, ok := chromiumBrowsers[browser]; ok {
    profiles, err := chromium.ListProfiles(b)
    if err != nil {
      return err
    }
    printChromiumProfiles(profiles)
    return nil
  }

  kind, err := classifyPath(browser)
  if err != nil {
    return err
  }
  switch kind {
  case kindFirefoxRoot:
    profiles, err := firefox.ListProfilesAt(browser)
    if err != nil {
      return err
    }
    printFirefoxProfiles(profiles)
  case kindChromiumRoot:
    profiles, err := chromium.ListProfilesAt(browser)
    if err != nil {
      return err
    }
    printChromiumProfiles(profiles)
  default:
    return fmt.Errorf("%q is a single profile, not a directory of profiles", browser)
  }
  return nil
}

func printFirefoxProfiles(profiles []firefox.Profile) {
  for _, p := range profiles {
    marker := ""
    if p.Default {
      marker = " (default)"
    }
    fmt.Printf("%s%s\t%s\n", p.Name, marker, p.Path)
  }
}

func printChromiumProfiles(profiles []chromium.ProfileInfo) {
  for _, p := range profiles {
    if p.Name != "" {
      fmt.Printf("%s\t%s\n", p.Dir, p.Name)
    } else {
      fmt.Println(p.Dir)
    }
  }
}

func runExtract(browser, profile, output string, dryRun bool) error {
  var root *model.Root
  var readPath string
  var err error

  switch {
  case firefoxBrowsers[browser] != "":
    root, readPath, err = extractFirefox(firefoxBrowsers[browser], profile)
  case chromiumBrowsers[browser] != "":
    p := profile
    if p == "" {
      p = "Default"
    }
    b := chromiumBrowsers[browser]
    var dir string
    dir, err = chromium.UserDataDir(b)
    if err == nil {
      readPath, err = chromium.ResolvePathAt(dir, p)
    }
    if err == nil {
      root, err = chromium.ReadFile(readPath)
    }
    if err == nil {
      root.Source = string(b)
      root.Profile = p
    }
  default:
    root, readPath, err = extractFromPath(browser, profile)
  }
  if err != nil {
    return err
  }

  if dryRun {
    dryRunHeader()
    fmt.Printf("Browser:       %s\n", browser)
    if profile != "" {
      fmt.Printf("Profile:       %s\n", profile)
    }
    if root.Profile != "" && root.Profile != profile {
      fmt.Printf("Resolved to:   %s\n", root.Profile)
    }
    fmt.Printf("Source format: %s\n", root.Format)
    fmt.Printf("Reading from:  %s\n", readPath)
    fmt.Println()
    printBookmarkSummary(root)
    fmt.Println()
    if output == "" {
      fmt.Println("Would write to: stdout")
    } else {
      fmt.Printf("Would write to: %s\n", output)
    }
    return nil
  }

  data, err := json.MarshalIndent(root, "", "  ")
  if err != nil {
    return fmt.Errorf("encoding output: %w", err)
  }
  data = append(data, '\n')

  if output == "" {
    if _, err := os.Stdout.Write(data); err != nil {
      return fmt.Errorf("writing to stdout: %w", err)
    }
    return nil
  }
  if err := os.WriteFile(output, data, 0o644); err != nil {
    return fmt.Errorf("writing %q: %w", output, err)
  }
  return nil
}

func extractFirefox(product firefox.Product, profile string) (*model.Root, string, error) {
  p, err := firefox.ResolveProfile(product, profile)
  if err != nil {
    return nil, "", err
  }
  root, err := firefox.Read(product, p.Path)
  return root, filepath.Join(p.Path, "places.sqlite"), err
}

// extractFromPath handles --browser values that aren't a recognized name:
// it treats the value as a filesystem path and auto-detects whether it's a
// Chromium Bookmarks file/profile/user-data-root or a Firefox
// places.sqlite/profile/profiles.ini-root. It returns the resolved root
// plus the exact file path that was (or would be) read from.
func extractFromPath(path, profile string) (*model.Root, string, error) {
  kind, err := classifyPath(path)
  if err != nil {
    return nil, "", err
  }

  switch kind {
  case kindChromiumFile:
    root, err := chromium.ReadFile(path)
    if err != nil {
      return nil, "", err
    }
    return withCustomSource(root, filepath.Dir(path)), path, nil

  case kindFirefoxFile:
    root, err := firefox.ReadProfile(filepath.Dir(path))
    if err != nil {
      return nil, "", err
    }
    return withCustomSource(root, filepath.Dir(path)), path, nil

  case kindFirefoxProfileDir:
    root, err := firefox.ReadProfile(path)
    readPath := filepath.Join(path, "places.sqlite")
    if err != nil {
      return nil, readPath, err
    }
    return withCustomSource(root, path), readPath, nil

  case kindChromiumProfileDir:
    readPath := filepath.Join(path, "Bookmarks")
    root, err := chromium.ReadFile(readPath)
    if err != nil {
      return nil, readPath, err
    }
    return withCustomSource(root, path), readPath, nil

  case kindFirefoxRoot:
    p, err := firefox.ResolveProfileAt(path, profile)
    if err != nil {
      return nil, "", err
    }
    readPath := filepath.Join(p.Path, "places.sqlite")
    root, err := firefox.ReadProfile(p.Path)
    if err != nil {
      return nil, readPath, err
    }
    return withCustomSource(root, p.Path), readPath, nil

  case kindChromiumRoot:
    p := profile
    if p == "" {
      p = "Default"
    }
    resolvedPath, err := chromium.ResolvePathAt(path, p)
    if err != nil {
      return nil, "", err
    }
    root, err := chromium.ReadFile(resolvedPath)
    if err != nil {
      return nil, resolvedPath, err
    }
    return withCustomSource(root, filepath.Dir(resolvedPath)), resolvedPath, nil
  }

  return nil, "", fmt.Errorf("could not detect a bookmarks store under %q", path)
}

func withCustomSource(root *model.Root, profilePath string) *model.Root {
  root.Source = "custom"
  root.Profile = profilePath
  return root
}
