package chromium

import (
  "encoding/json"
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
  Chrome   Browser = "chrome"
  Chromium Browser = "chromium"
  Brave    Browser = "brave"
  Edge     Browser = "edge"
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
    case Chromium:
      return filepath.Join(base, "Chromium", "User Data"), nil
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
    case Chromium:
      return filepath.Join(base, "Chromium"), nil
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
    case Chromium:
      return filepath.Join(base, "chromium"), nil
    case Brave:
      return filepath.Join(base, "BraveSoftware", "Brave-Browser"), nil
    case Edge:
      return filepath.Join(base, "microsoft-edge"), nil
    }
  }
  return "", fmt.Errorf("unsupported browser: %s", b)
}

// ProfileInfo is one profile directory, with its display name if known.
type ProfileInfo struct {
  Dir  string // e.g. "Default", "Profile 1"
  Name string // display name shown in the browser's profile picker, e.g. "Eduardo Sanchez"
}

// ListProfiles returns the profiles for the given browser.
func ListProfiles(b Browser) ([]ProfileInfo, error) {
  dir, err := UserDataDir(b)
  if err != nil {
    return nil, err
  }
  return ListProfilesAt(dir)
}

// ListProfilesAt returns the profiles under an arbitrary Chromium-style
// user-data directory (e.g. a custom install location). It prefers the
// "Local State" file's profile.info_cache, which lists every profile the
// browser knows about (including ones with no bookmarks yet); if that file
// is missing or unreadable, it falls back to scanning for subdirectories
// that already contain a Bookmarks file.
func ListProfilesAt(dir string) ([]ProfileInfo, error) {
  if names, err := readLocalStateNames(dir); err == nil && len(names) > 0 {
    var profiles []ProfileInfo
    for d, name := range names {
      profiles = append(profiles, ProfileInfo{Dir: d, Name: name})
    }
    sort.Slice(profiles, func(i, j int) bool { return profiles[i].Dir < profiles[j].Dir })
    return profiles, nil
  }

  entries, err := os.ReadDir(dir)
  if err != nil {
    return nil, fmt.Errorf("reading user data dir %q: %w", dir, err)
  }
  var profiles []ProfileInfo
  for _, e := range entries {
    if !e.IsDir() {
      continue
    }
    if _, err := os.Stat(filepath.Join(dir, e.Name(), "Bookmarks")); err == nil {
      profiles = append(profiles, ProfileInfo{Dir: e.Name()})
    }
  }
  sort.Slice(profiles, func(i, j int) bool { return profiles[i].Dir < profiles[j].Dir })
  return profiles, nil
}

// readLocalStateNames reads the "Local State" file (sibling to the profile
// directories) and returns a map of profile directory name to display name.
func readLocalStateNames(dir string) (map[string]string, error) {
  data, err := os.ReadFile(filepath.Join(dir, "Local State"))
  if err != nil {
    return nil, err
  }
  var state struct {
    Profile struct {
      InfoCache map[string]struct {
        Name string `json:"name"`
      } `json:"info_cache"`
    } `json:"profile"`
  }
  if err := json.Unmarshal(data, &state); err != nil {
    return nil, fmt.Errorf("parsing Local State: %w", err)
  }
  names := make(map[string]string, len(state.Profile.InfoCache))
  for d, info := range state.Profile.InfoCache {
    names[d] = info.Name
  }
  return names, nil
}

// ResolveProfileDir resolves a --profile value to an actual profile
// directory name under dir. nameOrDir may be a literal directory name
// (e.g. "Profile 1") or a display name shown in the browser's profile
// picker (e.g. "Eduardo Sanchez").
func ResolveProfileDir(dir, nameOrDir string) (string, error) {
  profiles, err := ListProfilesAt(dir)
  if err != nil {
    return "", err
  }
  for _, p := range profiles {
    if p.Dir == nameOrDir {
      return p.Dir, nil
    }
  }
  for _, p := range profiles {
    if p.Name != "" && p.Name == nameOrDir {
      return p.Dir, nil
    }
  }
  // Last resort: accept it as a literal directory name even if it wasn't
  // listed (e.g. a brand new profile Local State hasn't recorded yet).
  if info, err := os.Stat(filepath.Join(dir, nameOrDir)); err == nil && info.IsDir() {
    return nameOrDir, nil
  }
  return "", fmt.Errorf("no profile named or directoried %q found under %q", nameOrDir, dir)
}

// ResolvePath resolves the given browser/profile to the profile's
// Bookmarks file path, without reading or writing it.
func ResolvePath(b Browser, profile string) (string, error) {
  dir, err := UserDataDir(b)
  if err != nil {
    return "", err
  }
  return ResolvePathAt(dir, profile)
}

// ResolvePathAt resolves a profile under an arbitrary Chromium-style
// user-data directory to its Bookmarks file path.
func ResolvePathAt(dir, profile string) (string, error) {
  resolved, err := ResolveProfileDir(dir, profile)
  if err != nil {
    return "", err
  }
  return filepath.Join(dir, resolved, "Bookmarks"), nil
}
