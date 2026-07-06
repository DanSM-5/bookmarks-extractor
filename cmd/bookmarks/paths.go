package main

import (
  "fmt"
  "os"
  "path/filepath"

  "bookmarks/internal/chromium"
)

// pathKind classifies an arbitrary filesystem path passed as --browser (or
// import's --browser) into the specific bookmark-store layout found there.
type pathKind int

const (
  kindUnknown pathKind = iota
  kindChromiumFile
  kindChromiumProfileDir
  kindChromiumRoot
  kindFirefoxFile
  kindFirefoxProfileDir
  kindFirefoxRoot
)

// classifyPath inspects path and determines which bookmark-store layout it
// looks like, trying the most specific match first.
func classifyPath(path string) (pathKind, error) {
  info, err := os.Stat(path)
  if err != nil {
    return kindUnknown, fmt.Errorf("%q is not a known browser and not a valid path: %w", path, err)
  }

  if !info.IsDir() {
    switch filepath.Base(path) {
    case "Bookmarks":
      return kindChromiumFile, nil
    case "places.sqlite":
      return kindFirefoxFile, nil
    }
    return kindUnknown, fmt.Errorf("don't know how to handle file %q (expected a Bookmarks or places.sqlite file)", path)
  }

  if fileExists(filepath.Join(path, "places.sqlite")) {
    return kindFirefoxProfileDir, nil
  }
  if fileExists(filepath.Join(path, "Bookmarks")) {
    return kindChromiumProfileDir, nil
  }
  if fileExists(filepath.Join(path, "profiles.ini")) {
    return kindFirefoxRoot, nil
  }
  if profiles, err := chromium.ListProfilesAt(path); err == nil && len(profiles) > 0 {
    return kindChromiumRoot, nil
  }

  return kindUnknown, fmt.Errorf(
    "could not detect a bookmarks store under %q (looked for places.sqlite, Bookmarks, profiles.ini, or profile subdirectories)", path)
}

func fileExists(path string) bool {
  _, err := os.Stat(path)
  return err == nil
}
