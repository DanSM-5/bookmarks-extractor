package firefox

import (
  "bufio"
  "fmt"
  "os"
  "path/filepath"
  "runtime"
  "strings"
)

// Profile is one entry from Firefox's profiles.ini.
type Profile struct {
  Name    string
  Path    string // absolute path to the profile directory
  Default bool
}

// RootDir returns the directory containing profiles.ini and the profile
// folders for the current OS.
func RootDir() (string, error) {
  home, err := os.UserHomeDir()
  if err != nil {
    return "", err
  }

  switch runtime.GOOS {
  case "windows":
    base := os.Getenv("APPDATA")
    if base == "" {
      base = filepath.Join(home, "AppData", "Roaming")
    }
    return filepath.Join(base, "Mozilla", "Firefox"), nil
  case "darwin":
    return filepath.Join(home, "Library", "Application Support", "Firefox"), nil
  default:
    return filepath.Join(home, ".mozilla", "firefox"), nil
  }
}

// ListProfiles parses profiles.ini and returns all declared profiles with
// paths resolved to absolute.
func ListProfiles() ([]Profile, error) {
  root, err := RootDir()
  if err != nil {
    return nil, err
  }
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
    return nil, err
  }
  return profiles, nil
}

// DefaultProfile returns the profile marked Default in profiles.ini, or the
// first profile if none is marked.
func DefaultProfile() (Profile, error) {
  profiles, err := ListProfiles()
  if err != nil {
    return Profile{}, err
  }
  if len(profiles) == 0 {
    return Profile{}, fmt.Errorf("no Firefox profiles found")
  }
  for _, p := range profiles {
    if p.Default {
      return p, nil
    }
  }
  return profiles[0], nil
}
