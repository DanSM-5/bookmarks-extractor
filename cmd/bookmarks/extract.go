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
  cmd.Flags().StringVar(&profile, "profile", "", "profile name (chromium: directory or display name, e.g. \"Default\" or \"Eduardo Sanchez\"; firefox: profile name from profiles.ini). Defaults to the browser's default profile")
  cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (defaults to stdout)")
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
  kind, err := classifyPath(path)
  if err != nil {
    return nil, err
  }

  switch kind {
  case kindChromiumFile:
    root, err := chromium.ReadFile(path)
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, filepath.Dir(path)), nil

  case kindFirefoxFile:
    root, err := firefox.ReadProfile(filepath.Dir(path))
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, filepath.Dir(path)), nil

  case kindFirefoxProfileDir:
    root, err := firefox.ReadProfile(path)
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, path), nil

  case kindChromiumProfileDir:
    root, err := chromium.ReadFile(filepath.Join(path, "Bookmarks"))
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, path), nil

  case kindFirefoxRoot:
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

  case kindChromiumRoot:
    p := profile
    if p == "" {
      p = "Default"
    }
    resolvedPath, err := chromium.ResolvePathAt(path, p)
    if err != nil {
      return nil, err
    }
    root, err := chromium.ReadFile(resolvedPath)
    if err != nil {
      return nil, err
    }
    return withCustomSource(root, filepath.Dir(resolvedPath)), nil
  }

  return nil, fmt.Errorf("could not detect a bookmarks store under %q", path)
}

func withCustomSource(root *model.Root, profilePath string) *model.Root {
  root.Source = "custom"
  root.Profile = profilePath
  return root
}
