// Package model defines the canonical bookmark tree format shared by all
// browser readers/writers.
package model

// NodeType identifies what kind of bookmark tree node this is.
type NodeType string

const (
  TypeBookmark  NodeType = "bookmark"
  TypeFolder    NodeType = "folder"
  TypeSeparator NodeType = "separator"
)

// Role identifies the special-cased meaning of a top-level root node
// (e.g. "this is the visible bookmarks bar"). It lets importers map roots
// across formats without relying on locale-dependent title text. It is only
// ever set on entries in Root.Roots, never on deeper descendants.
type Role string

const (
  RoleToolbar Role = "toolbar" // visible bookmarks bar
  RoleMenu    Role = "menu"    // Firefox's Bookmarks Menu (no Chromium equivalent)
  RoleOther   Role = "other"   // catch-all/unsorted (Chrome "Other bookmarks", Firefox "unfiled")
  RoleMobile  Role = "mobile"  // mobile bookmarks
  RoleSynced  Role = "synced"  // Chromium-only extra sync root; rare
)

// Node is one entry in a bookmark tree: a bookmark, a folder, or a
// separator. ID is the browser-native GUID, preserved verbatim so the same
// bookmark can be recognized across exports from the same profile.
type Node struct {
  ID           string   `json:"id"`
  Type         NodeType `json:"type"`
  Role         Role     `json:"role,omitempty"`
  Title        string   `json:"title,omitempty"`
  URL          string   `json:"url,omitempty"`
  Index        int      `json:"index"`
  DateAdded    int64    `json:"dateAdded,omitempty"`    // unix millis
  DateModified int64    `json:"dateModified,omitempty"` // unix millis
  Children     []*Node  `json:"children,omitempty"`
}

// Format identifies the on-disk bookmark storage format a Root was read
// from (or should be written to), independent of the specific browser.
type Format string

const (
  FormatChromium Format = "chromium"
  FormatFirefox  Format = "firefox"
)

// Root is the top-level container for a full bookmark export: the roots
// (e.g. "Bookmarks Toolbar", "Other Bookmarks") plus metadata about where
// the export came from.
type Root struct {
  Source  string  `json:"source"` // e.g. "firefox", "chrome", "brave", "edge", "custom"
  Format  Format  `json:"format"` // "chromium" or "firefox" - the on-disk layout
  Profile string  `json:"profile"`
  Roots   []*Node `json:"roots"`
}
