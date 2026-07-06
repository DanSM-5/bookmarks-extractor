// Package netscape writes the "Netscape Bookmark File" format: the de
// facto standard every major browser can import via its own bookmarks UI
// (Firefox: Import and Backup > Import Bookmarks from HTML File...; Chrome/
// Brave/Edge: bookmark manager > Import bookmarks). Unlike writing a
// browser's native storage directly, importing this way merges into
// whatever bookmarks already exist instead of replacing them, so it's used
// as the safe default import path.
package netscape

import (
  "fmt"
  "html"
  "io"

  "bookmarks/internal/model"
)

// prettyRootTitles maps the raw internal titles Firefox stores for its
// special root folders to friendlier display names.
var prettyRootTitles = map[string]string{
  "menu":    "Bookmarks Menu",
  "toolbar": "Bookmarks Toolbar",
  "unfiled": "Other Bookmarks",
  "mobile":  "Mobile Bookmarks",
}

// Write emits a Netscape Bookmark File for root to w.
func Write(root *model.Root, w io.Writer) error {
  fmt.Fprint(w, "<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
  fmt.Fprint(w, "<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
  fmt.Fprint(w, "<TITLE>Bookmarks</TITLE>\n")
  fmt.Fprint(w, "<H1>Bookmarks</H1>\n")
  fmt.Fprint(w, "<DL><p>\n")
  for _, r := range root.Roots {
    writeNode(w, r, true)
  }
  fmt.Fprint(w, "</DL><p>\n")
  return nil
}

func writeNode(w io.Writer, n *model.Node, topLevel bool) {
  switch n.Type {
  case model.TypeSeparator:
    fmt.Fprint(w, "<HR>\n")

  case model.TypeBookmark:
    fmt.Fprintf(w, "<DT><A HREF=\"%s\" ADD_DATE=\"%d\" LAST_MODIFIED=\"%d\">%s</A>\n",
      html.EscapeString(n.URL),
      millisToUnixSeconds(n.DateAdded),
      millisToUnixSeconds(n.DateModified),
      html.EscapeString(n.Title))

  default: // folder
    title := n.Title
    if topLevel {
      if pretty, ok := prettyRootTitles[title]; ok {
        title = pretty
      }
    }
    attr := ""
    if n.Role == model.RoleToolbar {
      // Recognized by some importers to place a folder's contents
      // directly onto the visible bookmarks bar; harmless where ignored.
      attr = ` PERSONAL_TOOLBAR_FOLDER="true"`
    }
    fmt.Fprintf(w, "<DT><H3 ADD_DATE=\"%d\" LAST_MODIFIED=\"%d\"%s>%s</H3>\n",
      millisToUnixSeconds(n.DateAdded), millisToUnixSeconds(n.DateModified), attr, html.EscapeString(title))
    fmt.Fprint(w, "<DL><p>\n")
    for _, c := range n.Children {
      writeNode(w, c, false)
    }
    fmt.Fprint(w, "</DL><p>\n")
  }
}

func millisToUnixSeconds(ms int64) int64 {
  return ms / 1000
}
