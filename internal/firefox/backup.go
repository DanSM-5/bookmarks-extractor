package firefox

import (
  "crypto/rand"
  "encoding/base64"
  "encoding/json"
  "fmt"
  "io"
  "regexp"

  "github.com/DanSM-5/bookmarks-extractor/internal/model"
)

// backupNode mirrors the shape of Firefox's own bookmark backup JSON (the
// same format written to <profile>/bookmarkbackups/bookmarks-*.jsonlz4, and
// readable uncompressed via Library > Import and Backup > Restore > Choose
// File...). Unlike the Netscape HTML importer, restoring from this format
// preserves root placement (toolbar/menu/other/mobile).
type backupNode struct {
  GUID         string       `json:"guid"`
  Title        string       `json:"title,omitempty"`
  ID           int          `json:"id"`
  TypeCode     int          `json:"typeCode"`
  Type         string       `json:"type"`
  Root         string       `json:"root,omitempty"`
  URI          string       `json:"uri,omitempty"`
  DateAdded    int64        `json:"dateAdded,omitempty"`
  LastModified int64        `json:"lastModified,omitempty"`
  Children     []backupNode `json:"children,omitempty"`
}

// firefoxGUIDPattern matches Firefox's own guid format: exactly 12
// URL-safe-base64 characters.
var firefoxGUIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{12}$`)

type backupBuilder struct {
  nextID           int
  regeneratedGUIDs int
}

func (b *backupBuilder) id() int {
  id := b.nextID
  b.nextID++
  return id
}

// guid returns id unchanged if it's already a valid Firefox guid (e.g. it
// came from a real Firefox/LibreWolf profile), otherwise generates a fresh
// one. Non-Firefox sources (Chrome's uuid-v4 guids, for instance) don't
// match Firefox's 12-character format and would fail its guid validation.
func (b *backupBuilder) guid(id string) string {
  if firefoxGUIDPattern.MatchString(id) {
    return id
  }
  b.regeneratedGUIDs++
  return newGUID()
}

// newGUID generates a guid the same way Firefox itself does:
// 9 random bytes, URL-safe base64 encoded, giving exactly 12 characters.
func newGUID() string {
  buf := make([]byte, 9)
  _, _ = rand.Read(buf)
  return base64.RawURLEncoding.EncodeToString(buf)
}

func (b *backupBuilder) node(n *model.Node) backupNode {
  raw := backupNode{
    ID:           b.id(),
    GUID:         b.guid(n.ID),
    Title:        n.Title,
    DateAdded:    n.DateAdded * 1000,
    LastModified: n.DateModified * 1000,
  }
  switch n.Type {
  case model.TypeBookmark:
    raw.TypeCode = typeBookmark
    raw.Type = "text/x-moz-place"
    raw.URI = n.URL
  case model.TypeSeparator:
    raw.TypeCode = typeSeparator
    raw.Type = "text/x-moz-place-separator"
  default:
    raw.TypeCode = typeFolder
    raw.Type = "text/x-moz-place-container"
    for _, c := range n.Children {
      raw.Children = append(raw.Children, b.node(c))
    }
  }
  return raw
}

// rootFolder builds one of the four fixed top-level containers Firefox
// requires (menu/toolbar/unfiled/mobile), using its well-known guid so
// Firefox recognizes it as that specific root on restore.
func (b *backupBuilder) rootFolder(guid, title, root string, children []*model.Node) backupNode {
  node := backupNode{
    ID:       b.id(),
    GUID:     guid,
    Title:    title,
    TypeCode: typeFolder,
    Type:     "text/x-moz-place-container",
    Root:     root,
  }
  for _, c := range children {
    node.Children = append(node.Children, b.node(c))
  }
  return node
}

// BuildBackup converts a canonical Root into Firefox's bookmark backup
// tree, along with any warnings about lossy conversions (regenerated
// guids, unrecognized roots folded into "Other Bookmarks").
func BuildBackup(root *model.Root) (*backupNode, []string) {
  b := &backupBuilder{nextID: 2} // id 1 is reserved for the placesRoot itself

  var toolbar, menu, other, mobile *model.Node
  var unclassified []*model.Node
  for _, r := range root.Roots {
    switch r.Role {
    case model.RoleToolbar:
      toolbar = r
    case model.RoleMenu:
      menu = r
    case model.RoleOther:
      other = r
    case model.RoleMobile:
      mobile = r
    default:
      unclassified = append(unclassified, r)
    }
  }

  var warnings []string
  var otherChildren []*model.Node
  if other != nil {
    otherChildren = append(otherChildren, other.Children...)
  }
  for _, u := range unclassified {
    otherChildren = append(otherChildren, u)
    warnings = append(warnings, fmt.Sprintf("nested unrecognized root %q under \"Other Bookmarks\"", u.Title))
  }

  var toolbarChildren, menuChildren, mobileChildren []*model.Node
  if toolbar != nil {
    toolbarChildren = toolbar.Children
  }
  if menu != nil {
    menuChildren = menu.Children
  }
  if mobile != nil {
    mobileChildren = mobile.Children
  }

  placesRoot := &backupNode{
    ID:       1,
    GUID:     guidRoot,
    TypeCode: typeFolder,
    Type:     "text/x-moz-place-container",
    Root:     "placesRoot",
    Children: []backupNode{
      b.rootFolder(guidMenu, "Bookmarks Menu", "bookmarksMenuFolder", menuChildren),
      b.rootFolder(guidToolbar, "Bookmarks Toolbar", "toolbarFolder", toolbarChildren),
      b.rootFolder(guidUnfiled, "Other Bookmarks", "unfiledBookmarksFolder", otherChildren),
      b.rootFolder(guidMobile, "Mobile Bookmarks", "mobileFolder", mobileChildren),
    },
  }

  if b.regeneratedGUIDs > 0 {
    warnings = append(warnings, fmt.Sprintf(
      "regenerated %d bookmark ID(s) that weren't valid Firefox guids (expected when importing from a different browser)",
      b.regeneratedGUIDs))
  }

  return placesRoot, warnings
}

// WriteBackup writes root to w as a Firefox bookmark backup JSON document.
func WriteBackup(root *model.Root, w io.Writer) ([]string, error) {
  backup, warnings := BuildBackup(root)
  data, err := json.MarshalIndent(backup, "", "  ")
  if err != nil {
    return warnings, fmt.Errorf("encoding bookmarks backup: %w", err)
  }
  if _, err := w.Write(data); err != nil {
    return warnings, fmt.Errorf("writing bookmarks backup: %w", err)
  }
  return warnings, nil
}
