package chromium

import (
  "fmt"
  "os"
  "path/filepath"
  "runtime"
  "sort"
)

// Browser identifies a specific Chromium-based browser. They all share the
// same "Bookmarks" JSON layout, so only the user-data directory differs.
type Browser string

const (
  Chrome Browser = "chrome"
  Brave  Browser = "brave"
  Edge   Browser = "edge"
)

// UserDataDir returns the browser's root profile directory for the current OS.
func UserDataDir(b Browser) (string, error) {
  home, err := os.UserHomeDir()
  if err != nil {
    return "", err
  }

  switch runtime.GOOS {
  case "windows":
    base := os.Getenv("LOCALAPPDATA")
    if base == "" {
      base = filepath.Join(home, "AppData", "Local")
    }
    switch b {
    case Chrome:
      return filepath.Join(base, "Google", "Chrome", "User Data"), nil
    case Brave:
      return filepath.Join(base, "BraveSoftware", "Brave-Browser", "User Data"), nil
    case Edge:
      return filepath.Join(base, "Microsoft", "Edge", "User Data"), nil
    }
  case "darwin":
    base := filepath.Join(home, "Library", "Application Support")
    switch b {
    case Chrome:
      return filepath.Join(base, "Google", "Chrome"), nil
    case Brave:
      return filepath.Join(base, "BraveSoftware", "Brave-Browser"), nil
    case Edge:
      return filepath.Join(base, "Microsoft Edge"), nil
    }
  default: // linux and other unix-likes
    base := filepath.Join(home, ".config")
    switch b {
    case Chrome:
      return filepath.Join(base, "google-chrome"), nil
    case Brave:
      return filepath.Join(base, "BraveSoftware", "Brave-Browser"), nil
    case Edge:
      return filepath.Join(base, "microsoft-edge"), nil
    }
  }
  return "", fmt.Errorf("unsupported browser: %s", b)
}

// ListProfiles returns the profile directory names (e.g. "Default",
// "Profile 1") that contain a Bookmarks file.
func ListProfiles(b Browser) ([]string, error) {
  dir, err := UserDataDir(b)
  if err != nil {
    return nil, err
  }
  entries, err := os.ReadDir(dir)
  if err != nil {
    return nil, fmt.Errorf("reading user data dir %q: %w", dir, err)
  }

  var profiles []string
  for _, e := range entries {
    if !e.IsDir() {
      continue
    }
    if _, err := os.Stat(filepath.Join(dir, e.Name(), "Bookmarks")); err == nil {
      profiles = append(profiles, e.Name())
    }
  }
  sort.Strings(profiles)
  return profiles, nil
}
