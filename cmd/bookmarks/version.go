package main

import (
  _ "embed"
  "fmt"
  "runtime/debug"
  "strings"
)

// semver is the semantic-version part of the build, read from version.txt
// at compile time. Bump this file to cut a new version.
//
//go:embed version.txt
var semver string

// fullVersion returns "<semver>@<commit>", e.g. "0.0.0@a1b2c3d", with
// "-dirty" appended if the working tree had uncommitted changes at build
// time. The commit comes from runtime/debug.ReadBuildInfo(), which the Go
// toolchain stamps into the binary automatically when building from a git
// checkout - no ldflags or build script required.
func fullVersion() string {
  version := strings.TrimSpace(semver)

  sha := "unknown"
  dirty := false
  if info, ok := debug.ReadBuildInfo(); ok {
    for _, s := range info.Settings {
      switch s.Key {
      case "vcs.revision":
        sha = s.Value
        if len(sha) > 7 {
          sha = sha[:7]
        }
      case "vcs.modified":
        dirty = s.Value == "true"
      }
    }
  }
  if dirty {
    sha += "-dirty"
  }

  return fmt.Sprintf("%s@%s", version, sha)
}
