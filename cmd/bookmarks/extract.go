package main

import (
  "encoding/json"
  "fmt"
  "os"

  "github.com/spf13/cobra"

  "bookmarks/internal/chromium"
  "bookmarks/internal/firefox"
  "bookmarks/internal/model"
)

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

  cmd.Flags().StringVar(&browser, "browser", "", "browser to extract from: firefox, chrome, brave, edge (required)")
  cmd.Flags().StringVar(&profile, "profile", "", "profile name (chromium: e.g. \"Default\", \"Profile 1\"; firefox: profile name from profiles.ini). Defaults to the browser's default profile")
  cmd.Flags().StringVar(&output, "output", "", "output file path (defaults to stdout)")
  cmd.Flags().BoolVar(&listProfiles, "list-profiles", false, "list available profiles for --browser and exit")
  cmd.MarkFlagRequired("browser")

  return cmd
}

func runListProfiles(browser string) error {
  switch browser {
  case "firefox":
    profiles, err := firefox.ListProfiles()
    if err != nil {
      return err
    }
    for _, p := range profiles {
      marker := ""
      if p.Default {
        marker = " (default)"
      }
      fmt.Printf("%s%s\t%s\n", p.Name, marker, p.Path)
    }
  case "chrome", "brave", "edge":
    names, err := chromium.ListProfiles(chromium.Browser(browser))
    if err != nil {
      return err
    }
    for _, n := range names {
      fmt.Println(n)
    }
  default:
    return fmt.Errorf("unknown browser %q (expected firefox, chrome, brave, edge)", browser)
  }
  return nil
}

func runExtract(browser, profile, output string) error {
  var root *model.Root
  var err error

  switch browser {
  case "firefox":
    root, err = extractFirefox(profile)
  case "chrome", "brave", "edge":
    if profile == "" {
      profile = "Default"
    }
    root, err = chromium.Read(chromium.Browser(browser), profile)
  default:
    return fmt.Errorf("unknown browser %q (expected firefox, chrome, brave, edge)", browser)
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

func extractFirefox(profile string) (*model.Root, error) {
  var profilePath string
  if profile == "" {
    p, err := firefox.DefaultProfile()
    if err != nil {
      return nil, err
    }
    profilePath = p.Path
  } else {
    profiles, err := firefox.ListProfiles()
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
      // Fall back to treating --profile as a literal path.
      profilePath = profile
    }
  }
  return firefox.Read(profilePath)
}
