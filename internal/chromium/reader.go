package chromium

import (
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "strconv"

  "bookmarks/internal/model"
)

// chromeEpochOffsetMicros is the number of microseconds between the WebKit
// epoch (1601-01-01) and the Unix epoch (1970-01-01), used to decode
// date_added/date_modified.
const chromeEpochOffsetMicros = 11644473600000000

type rawFile struct {
  Roots   map[string]rawNode `json:"roots"`
  Version int                `json:"version"`
}

type rawNode struct {
  Children     []rawNode `json:"children"`
  DateAdded    string    `json:"date_added"`
  DateModified string    `json:"date_modified"`
  GUID         string    `json:"guid"`
  ID           string    `json:"id"`
  Name         string    `json:"name"`
  Type         string    `json:"type"` // "url" or "folder"
  URL          string    `json:"url"`
}

// rootOrder controls the order in which known roots are emitted; any
// unrecognized root keys are appended afterwards.
var rootOrder = []string{"bookmark_bar", "other", "synced", "mobile"}

// ReadFile parses a Chromium "Bookmarks" JSON file into the canonical model.
func ReadFile(path string) (*model.Root, error) {
  data, err := os.ReadFile(path)
  if err != nil {
    return nil, fmt.Errorf("reading bookmarks file %q: %w", path, err)
  }

  var raw rawFile
  if err := json.Unmarshal(data, &raw); err != nil {
    return nil, fmt.Errorf("parsing bookmarks file %q: %w", path, err)
  }

  root := &model.Root{}
  seen := map[string]bool{}
  for _, key := range rootOrder {
    n, ok := raw.Roots[key]
    if !ok {
      continue
    }
    seen[key] = true
    root.Roots = append(root.Roots, convertNode(n, 0))
  }
  for key, n := range raw.Roots {
    if seen[key] {
      continue
    }
    root.Roots = append(root.Roots, convertNode(n, 0))
  }

  return root, nil
}

func convertNode(n rawNode, index int) *model.Node {
  node := &model.Node{
    ID:           firstNonEmpty(n.GUID, n.ID),
    Title:        n.Name,
    Index:        index,
    DateAdded:    chromeTimestampToUnixMillis(n.DateAdded),
    DateModified: chromeTimestampToUnixMillis(n.DateModified),
  }

  if n.Type == "url" {
    node.Type = model.TypeBookmark
    node.URL = n.URL
    return node
  }

  node.Type = model.TypeFolder
  for i, child := range n.Children {
    node.Children = append(node.Children, convertNode(child, i))
  }
  return node
}

func chromeTimestampToUnixMillis(s string) int64 {
  if s == "" {
    return 0
  }
  micros, err := strconv.ParseInt(s, 10, 64)
  if err != nil || micros == 0 {
    return 0
  }
  return (micros - chromeEpochOffsetMicros) / 1000
}

func firstNonEmpty(vals ...string) string {
  for _, v := range vals {
    if v != "" {
      return v
    }
  }
  return ""
}

// Read locates and parses the Bookmarks file for the given browser/profile.
func Read(b Browser, profile string) (*model.Root, error) {
  dir, err := UserDataDir(b)
  if err != nil {
    return nil, err
  }
  path := filepath.Join(dir, profile, "Bookmarks")
  root, err := ReadFile(path)
  if err != nil {
    return nil, err
  }
  root.Source = string(b)
  root.Profile = profile
  return root, nil
}
