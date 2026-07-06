package main

import (
  "fmt"

  "github.com/DanSM-5/bookmarks-extractor/internal/model"
)

// flatBookmark is a bookmark flattened out of the tree, for display.
type flatBookmark struct {
  Title string
  URL   string
}

// treeSummary is a count/preview of a bookmark tree (or subtree).
type treeSummary struct {
  Bookmarks int
  Folders   int
  First     []flatBookmark // up to the requested limit, in depth-first order
}

// summarizeNodes walks nodes depth-first and tallies bookmarks/folders,
// collecting up to limit bookmarks into First. limit < 0 means unlimited;
// limit == 0 collects none (counts only).
func summarizeNodes(nodes []*model.Node, limit int) treeSummary {
  var s treeSummary
  var walk func(n *model.Node)
  walk = func(n *model.Node) {
    switch n.Type {
    case model.TypeBookmark:
      s.Bookmarks++
      if limit < 0 || len(s.First) < limit {
        s.First = append(s.First, flatBookmark{Title: n.Title, URL: n.URL})
      }
    case model.TypeFolder:
      s.Folders++
    }
    for _, c := range n.Children {
      walk(c)
    }
  }
  for _, n := range nodes {
    walk(n)
  }
  return s
}

// printBookmarkSummary prints a per-root breakdown, totals, and the first
// 10 bookmarks (depth-first across all roots) with an ellipsis if more
// exist.
func printBookmarkSummary(root *model.Root) {
  fmt.Println("Roots:")
  for _, r := range root.Roots {
    // Summarize r's children, not r itself, so the root container (e.g.
    // "Mobile bookmarks") isn't counted as a folder of its own contents.
    s := summarizeNodes(r.Children, 0)
    label := r.Title
    if label == "" {
      label = "(untitled)"
    }
    if r.Role != "" {
      label = fmt.Sprintf("%s [%s]", label, r.Role)
    }
    fmt.Printf("  %-40s %5d bookmark(s), %4d folder(s)\n", label, s.Bookmarks, s.Folders)
  }

  const previewLimit = 10
  var allChildren []*model.Node
  for _, r := range root.Roots {
    allChildren = append(allChildren, r.Children...)
  }
  total := summarizeNodes(allChildren, previewLimit)
  fmt.Println()
  fmt.Printf("Total: %d bookmark(s) in %d folder(s)\n", total.Bookmarks, total.Folders)
  for i, b := range total.First {
    title := b.Title
    if title == "" {
      title = "(untitled)"
    }
    fmt.Printf("  %2d. %s\n      %s\n", i+1, title, b.URL)
  }
  if total.Bookmarks > len(total.First) {
    fmt.Println("  ...")
  }
}

func dryRunHeader() {
  fmt.Println("Dry run - no files will be written and no bookmarks will be changed.")
  fmt.Println()
}
