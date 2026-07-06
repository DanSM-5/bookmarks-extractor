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

  cmd := &cobra.Command{
    Use:   "extract",
    Short: "Extract bookmarks from a browser profile into the canonical JSON format",
    RunE: func(cmd *cobra.Command, args []string) error {
      if listProfiles {
        return runListProfiles(browser)
      }
      return runExtract(browser, profile, output)
    },
  }

  cmd.Flags().StringVar(&browser, "browser", "",
    "browser to extract from: firefox, librewolf, chrome, chromium, brave, edge, "+
      "or a path to a custom install/profile location (required)")
  cmd.Flags().StringVar(&profile, "profile", "", "profile name (chromium: e.g. \"Default\", \"Profile 1\"; firefox: profile name from profiles.ini). Defaults to the browser's default profile")
  cmd.Flags().StringVar(&output, "output", "", "output file path (defaults to stdout)")
  cmd.Flags().BoolVar(&listProfiles, "list-profiles", false, "list available profiles for --browser and exit")
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
    names, err := chromium.ListProfiles(b)
    if err != nil {
      return err
    }
    for _, n := range names {
      fmt.Println(n)
    }
    return nil
  }
  return listProfilesAtPath(browser)
}

// listProfilesAtPath lists profiles for a custom (path-based) browser
// location, auto-detecting whether it looks like a Firefox-style root
// (profiles.ini) or a Chromium-style user-data root (profile subdirs).
func listProfilesAtPath(path string) error {
  info, err := os.Stat(path)
  if err != nil {
    return fmt.Errorf("%q is not a known browser and not a valid path: %w", path, err)
  }
  if !info.IsDir() {
    return fmt.Errorf("%q is a single profile, not a directory of profiles", path)
  }

  if fileExists(filepath.Join(path, "profiles.ini")) {
    profiles, err := firefox.ListProfilesAt(path)
    if err != nil {
      return err
    }
    printFirefoxProfiles(profiles)
    return nil
  }

  names, err := chromium.ListProfilesAt(path)
  if err != nil {
    return err
  }
  if len(names) == 0 {
    return fmt.Errorf("no profiles found under %q (expected profiles.ini or profile subdirectories containing a Bookmarks file)", path)
  }
  for _, n := range names {
    fmt.Println(n)
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

func runExtract(browser, profile, output string) error {
  var root *model.Root
  var err error

  switch {
  case firefoxBrowsers[browser] != "":
    root, err = extractFirefox(firefoxBrowsers[browser], profile)
  case chromiumBrowsers[browser] != "":
    if profile == "" {
      profile = "Default"
    }
    root, err = chromium.Read(chromiumBrowsers[browser], profile)
  default:
    root, err = extractFromPath(browser, profile)
  }
  if err != nil {
    return err
  }

  data, err := json.MarshalIndent(root, "", "  ")
  if err != nil {
    return fmt.Errorf("encoding output: %w", err)
  }
  data = append(data, '\n')

  if output == "" {
    _, err = os.Stdout.Write(data)
    return err
  }
  return os.WriteFile(output, data, 0o644)
}

func extractFirefox(product firefox.Product, profile string) (*model.Root, error) {
  var profilePath string
  if profile == "" {
    p, err := firefox.DefaultProfile(product)
    if err != nil {
      return nil, err
    }
    profilePath = p.Path
  } else {
    if profiles, err := firefox.ListProfiles(product); err == nil {
      for _, p := range profiles {
        if p.Name == profile {
          profilePath = p.Path
          break
        }
      }
    }
    if profilePath == "" {
      // Fall back to treating --profile as a literal path.
      profilePath = profile
    }
  }
  return firefox.Read(product, profilePath)
}

// extractFromPath handles --browser values that aren't a recognized name:
// it treats the value as a filesystem path and auto-detects whether it's a
// Chromium Bookmarks file/profile/user-data-root or a Firefox
// places.sqlite/profile/profiles.ini-root.
func extractFromPath(path, profile string) (*model.Root, error) {
  info, err := os.Stat(path)
  if err != nil {
    return nil, fmt.Errorf("%q is not a known browser and not a valid path: %w", path, err)
  }

  if !info.IsDir() {
    switch filepath.Base(path) {
    case "Bookmarks":
      root, err := chromium.ReadFile(path)
      if err != nil {
        return nil, err
      }
      return withCustomSource(root, filepath.Dir(path)), nil
    case "places.sqlite":
      root, err := firefox.ReadProfile(filepath.Dir(path))
      if err != nil {
        return nil, err
      }
      return withCustomSource(root, filepath.Dir(path)), nil
    default:
      return nil, fmt.Errorf("don't know how to read bookmarks from file %q (expected a Bookmarks or places.sqlite file)", path)
    }
  }

  // Directory: try the most specific layout first.
  if fileExists(filepath.Join(path, "places.sqlite")) {
    root, err := firefox.ReadProfile(path)
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, path), nil
  }
  if fileExists(filepath.Join(path, "Bookmarks")) {
    root, err := chromium.ReadFile(filepath.Join(path, "Bookmarks"))
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, path), nil
  }
  if fileExists(filepath.Join(path, "profiles.ini")) {
    var profilePath string
    if profile == "" {
      p, err := firefox.DefaultProfileAt(path)
      if err != nil {
        return nil, err
      }
      profilePath = p.Path
    } else {
      profiles, err := firefox.ListProfilesAt(path)
      if err != nil {
        return nil, err
      }
      for _, p := range profiles {
        if p.Name == profile {
          profilePath = p.Path
          break
        }
      }
      if profilePath == "" {
        profilePath = filepath.Join(path, profile)
      }
    }
    root, err := firefox.ReadProfile(profilePath)
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, profilePath), nil
  }
  if names, err := chromium.ListProfilesAt(path); err == nil && len(names) > 0 {
    p := profile
    if p == "" {
      p = "Default"
    }
    root, err := chromium.ReadAt(path, p)
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, filepath.Join(path, p)), nil
  }

  return nil, fmt.Errorf("could not detect a bookmarks store under %q (looked for places.sqlite, Bookmarks, profiles.ini, or profile subdirectories)", path)
}

func withCustomSource(root *model.Root, profilePath string) *model.Root {
  root.Source = "custom"
  root.Profile = profilePath
  return root
}

func fileExists(path string) bool {
  _, err := os.Stat(path)
  return err == nil
}
