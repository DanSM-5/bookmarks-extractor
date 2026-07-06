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

// Node is one entry in a bookmark tree: a bookmark, a folder, or a
// separator. ID is the browser-native GUID, preserved verbatim so the same
// bookmark can be recognized across exports from the same profile.
type Node struct {
  ID           string   `json:"id"`
  Type         NodeType `json:"type"`
  Title        string   `json:"title,omitempty"`
  URL          string   `json:"url,omitempty"`
  Index        int      `json:"index"`
  DateAdded    int64    `json:"dateAdded,omitempty"`    // unix millis
  DateModified int64    `json:"dateModified,omitempty"` // unix millis
  Children     []*Node  `json:"children,omitempty"`
}

// Root is the top-level container for a full bookmark export: the roots
// (e.g. "Bookmarks Toolbar", "Other Bookmarks") plus metadata about where
// the export came from.
type Root struct {
  Source  string  `json:"source"` // e.g. "firefox", "chrome", "brave", "edge"
  Profile string  `json:"profile"`
  Roots   []*Node `json:"roots"`
}
