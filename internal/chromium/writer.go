package chromium

import (
  "encoding/json"
  "fmt"
  "os"
  "strconv"

  "github.com/google/uuid"

  "github.com/DanSM-5/bookmarks-extractor/internal/model"
)

// builder assembles a rawFile from a canonical model.Root, assigning
// sequential local IDs (as Chrome itself does) and tracking lossy
// conversions so callers can surface them as warnings.
type builder struct {
  nextID             int
  regeneratedGUIDs   int
  droppedSeparators  int
}

func (b *builder) id() string {
  id := strconv.Itoa(b.nextID)
  b.nextID++
  return id
}

// guid returns id if it's already a valid GUID (lowercased), otherwise
// generates a fresh one. Non-Chrome sources (e.g. Firefox's 12-character
// base64 guids) don't pass Chrome's GUID validation, so Chrome would
// silently regenerate them on load anyway.
func (b *builder) guid(id string) string {
  if parsed, err := uuid.Parse(id); err == nil {
    return parsed.String()
  }
  b.regeneratedGUIDs++
  return uuid.NewString()
}

func (b *builder) children(nodes []*model.Node) []rawNode {
  var out []rawNode
  for _, n := range nodes {
    if n.Type == model.TypeSeparator {
      b.droppedSeparators++
      continue
    }
    out = append(out, b.node(n))
  }
  return out
}

func (b *builder) node(n *model.Node) rawNode {
  raw := rawNode{
    ID:           b.id(),
    GUID:         b.guid(n.ID),
    Name:         n.Title,
    DateAdded:    unixMillisToChromeTimestamp(n.DateAdded),
    DateModified: unixMillisToChromeTimestamp(n.DateModified),
  }
  if n.Type == model.TypeBookmark {
    raw.Type = "url"
    raw.URL = n.URL
    return raw
  }
  raw.Type = "folder"
  raw.Children = b.children(n.Children)
  return raw
}

// folder builds a synthetic top-level root folder from a set of children
// that may originate from a different root entirely (e.g. Firefox's
// toolbar folder becoming Chrome's bookmark_bar).
func (b *builder) folder(title string, children []*model.Node) rawNode {
  raw := rawNode{
    ID:           b.id(),
    GUID:         uuid.NewString(),
    Name:         title,
    Type:         "folder",
    DateAdded:    unixMillisToChromeTimestamp(0),
    DateModified: unixMillisToChromeTimestamp(0),
  }
  raw.Children = b.children(children)
  return raw
}

// BuildFile converts a canonical Root into the Chromium "Bookmarks" JSON
// structure, along with any warnings about lossy conversions (dropped
// separators, regenerated GUIDs, unrecognized roots).
func BuildFile(root *model.Root) (*rawFile, []string) {
  b := &builder{nextID: 1}

  var toolbar, other, menu, mobile, synced *model.Node
  var unclassified []*model.Node
  for _, r := range root.Roots {
    switch r.Role {
    case model.RoleToolbar:
      toolbar = r
    case model.RoleOther:
      other = r
    case model.RoleMenu:
      menu = r
    case model.RoleMobile:
      mobile = r
    case model.RoleSynced:
      synced = r
    default:
      unclassified = append(unclassified, r)
    }
  }

  var warnings []string

  // Chrome has no equivalent of Firefox's Bookmarks Menu; fold it into
  // "Other bookmarks" as a named subfolder instead of dropping it.
  var otherChildren []*model.Node
  if other != nil {
    otherChildren = append(otherChildren, other.Children...)
  }
  if menu != nil && len(menu.Children) > 0 {
    title := menu.Title
    if title == "" || title == "menu" {
      title = "Bookmarks Menu"
    }
    otherChildren = append(otherChildren, &model.Node{Type: model.TypeFolder, Title: title, Children: menu.Children})
  }
  for _, u := range unclassified {
    otherChildren = append(otherChildren, u)
    warnings = append(warnings, fmt.Sprintf("nested unrecognized root %q under \"Other bookmarks\"", u.Title))
  }

  var toolbarChildren, mobileChildren []*model.Node
  if toolbar != nil {
    toolbarChildren = toolbar.Children
  }
  if mobile != nil {
    mobileChildren = append(mobileChildren, mobile.Children...)
  }
  // No real-world Chrome profile has been observed with a distinct
  // "synced" root separate from the mobile-bookmarks root (the mobile
  // root's own JSON key is, confusingly, "synced" - see rootOrder in
  // reader.go), but fold it in here too rather than dropping it in the
  // unlikely case some variant does emit one.
  if synced != nil {
    mobileChildren = append(mobileChildren, synced.Children...)
  }

  roots := map[string]rawNode{
    "bookmark_bar": b.folder("Bookmarks bar", toolbarChildren),
    "other":        b.folder("Other bookmarks", otherChildren),
    "synced":       b.folder("Mobile bookmarks", mobileChildren),
  }

  if b.regeneratedGUIDs > 0 {
    warnings = append(warnings, fmt.Sprintf(
      "regenerated %d bookmark ID(s) that weren't valid Chrome GUIDs (expected when importing from a different browser)",
      b.regeneratedGUIDs))
  }
  if b.droppedSeparators > 0 {
    warnings = append(warnings, fmt.Sprintf(
      "dropped %d separator(s): Chrome bookmarks don't support them", b.droppedSeparators))
  }

  return &rawFile{Roots: roots, Version: 1}, warnings
}

// WriteFile builds and writes a Chromium "Bookmarks" JSON file at path,
// overwriting anything already there. It does not back up the existing
// file - callers that care about that should do it before calling this.
func WriteFile(root *model.Root, path string) ([]string, error) {
  file, warnings := BuildFile(root)
  data, err := json.MarshalIndent(file, "", "  ")
  if err != nil {
    return warnings, fmt.Errorf("encoding bookmarks: %w", err)
  }
  if err := os.WriteFile(path, data, 0o644); err != nil {
    return warnings, fmt.Errorf("writing %q: %w", path, err)
  }
  return warnings, nil
}

func unixMillisToChromeTimestamp(ms int64) string {
  if ms == 0 {
    return "0"
  }
  micros := ms*1000 + chromeEpochOffsetMicros
  return strconv.FormatInt(micros, 10)
}
