package firefox

import (
  "bufio"
  "fmt"
  "os"
  "path/filepath"
  "runtime"
  "strings"
)

// Product identifies a specific Gecko-based browser. They all share the
// same profiles.ini + places.sqlite layout, so only the root directory
// differs.
type Product string

const (
  Firefox   Product = "firefox"
  LibreWolf Product = "librewolf"
)

// Profile is one entry from a product's profiles.ini.
type Profile struct {
  Name    string
  Path    string // absolute path to the profile directory
  Default bool
}

// RootDir returns the directory containing profiles.ini and the profile
// folders for the given product on the current OS.
func RootDir(p Product) (string, error) {
  home, err := os.UserHomeDir()
  if err != nil {
    return "", fmt.Errorf("determining home directory: %w", err)
  }

  switch runtime.GOOS {
  case "windows":
    base := os.Getenv("APPDATA")
    if base == "" {
      base = filepath.Join(home, "AppData", "Roaming")
    }
    switch p {
    case Firefox:
      return filepath.Join(base, "Mozilla", "Firefox"), nil
    case LibreWolf:
      return filepath.Join(base, "librewolf"), nil
    }
  case "darwin":
    base := filepath.Join(home, "Library", "Application Support")
    switch p {
    case Firefox:
      return filepath.Join(base, "Firefox"), nil
    case LibreWolf:
      return filepath.Join(base, "librewolf"), nil
    }
  default: // linux and other unix-likes
    switch p {
    case Firefox:
      return filepath.Join(home, ".mozilla", "firefox"), nil
    case LibreWolf:
      return filepath.Join(home, ".librewolf"), nil
    }
  }
  return "", fmt.Errorf("unsupported product: %s", p)
}

// ListProfiles parses profiles.ini for the given product and returns all
// declared profiles with paths resolved to absolute.
func ListProfiles(p Product) ([]Profile, error) {
  root, err := RootDir(p)
  if err != nil {
    return nil, err
  }
  return ListProfilesAt(root)
}

// ListProfilesAt parses profiles.ini under an arbitrary root directory
// (e.g. a custom install location) and returns all declared profiles.
func ListProfilesAt(root string) ([]Profile, error) {
  iniPath := filepath.Join(root, "profiles.ini")
  f, err := os.Open(iniPath)
  if err != nil {
    return nil, fmt.Errorf("opening %q: %w", iniPath, err)
  }
  defer f.Close()

  var profiles []Profile
  var cur *Profile
  var curIsRelative bool
  var curRawPath string

  flush := func() {
    if cur == nil {
      return
    }
    path := curRawPath
    if curIsRelative {
      path = filepath.Join(root, curRawPath)
    }
    cur.Path = path
    profiles = append(profiles, *cur)
  }

  scanner := bufio.NewScanner(f)
  for scanner.Scan() {
    line := strings.TrimSpace(scanner.Text())
    if line == "" {
      continue
    }
    if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
      flush()
      section := strings.Trim(line, "[]")
      if strings.HasPrefix(section, "Profile") {
        cur = &Profile{}
        curIsRelative = true
        curRawPath = ""
      } else {
        cur = nil
      }
      continue
    }
    if cur == nil {
      continue
    }
    key, val, ok := strings.Cut(line, "=")
    if !ok {
      continue
    }
    switch strings.TrimSpace(key) {
    case "Name":
      cur.Name = strings.TrimSpace(val)
    case "Path":
      curRawPath = strings.TrimSpace(val)
    case "IsRelative":
      curIsRelative = strings.TrimSpace(val) == "1"
    case "Default":
      cur.Default = strings.TrimSpace(val) == "1"
    }
  }
  flush()

  if err := scanner.Err(); err != nil {
    return nil, fmt.Errorf("reading %q: %w", iniPath, err)
  }
  return profiles, nil
}

// DefaultProfile returns the profile marked Default for the given product,
// or the first profile if none is marked.
func DefaultProfile(p Product) (Profile, error) {
  root, err := RootDir(p)
  if err != nil {
    return Profile{}, err
  }
  return DefaultProfileAt(root)
}

// DefaultProfileAt returns the profile marked Default under an arbitrary
// root directory, or the first profile if none is marked.
func DefaultProfileAt(root string) (Profile, error) {
  profiles, err := ListProfilesAt(root)
  if err != nil {
    return Profile{}, err
  }
  if len(profiles) == 0 {
    return Profile{}, fmt.Errorf("no profiles found under %q", root)
  }
  for _, p := range profiles {
    if p.Default {
      return p, nil
    }
  }
  return profiles[0], nil
}

// ResolveProfile resolves a --profile value to a specific profile for the
// given product: name is matched against profiles.ini profile names, or
// the default profile is returned if name is empty. It returns a clear
// error if name doesn't match any known profile, rather than silently
// falling back to treating it as something else.
func ResolveProfile(p Product, name string) (Profile, error) {
  root, err := RootDir(p)
  if err != nil {
    return Profile{}, err
  }
  return ResolveProfileAt(root, name)
}

// ResolveProfileAt is ResolveProfile for an arbitrary profiles.ini root
// directory (e.g. a custom install location).
func ResolveProfileAt(root, name string) (Profile, error) {
  if name == "" {
    return DefaultProfileAt(root)
  }
  profiles, err := ListProfilesAt(root)
  if err != nil {
    return Profile{}, err
  }
  for _, p := range profiles {
    if p.Name == name {
      return p, nil
    }
  }
  return Profile{}, fmt.Errorf("no profile named %q found under %q", name, root)
}
