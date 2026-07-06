package firefox

import (
  "database/sql"
  "fmt"
  "io"
  "os"
  "path/filepath"

  _ "modernc.org/sqlite"

  "bookmarks/internal/model"
)

// bookmarkType mirrors moz_bookmarks.type.
const (
  typeBookmark  = 1
  typeFolder    = 2
  typeSeparator = 3
)

// rootGUID is the guid of the synthetic top-level container in
// moz_bookmarks; its direct children are the real roots (menu, toolbar,
// unfiled, mobile).
const rootGUID = "root________"

// internalGUIDs are Firefox-internal containers that aren't real,
// user-facing bookmark folders and are excluded from the canonical tree.
var internalGUIDs = map[string]bool{
  "tags________": true,
}

// rootRoles maps the well-known guids of Firefox's top-level bookmark
// folders to their canonical Role.
var rootRoles = map[string]model.Role{
  "toolbar_____": model.RoleToolbar,
  "menu________": model.RoleMenu,
  "unfiled_____": model.RoleOther,
  "mobile______": model.RoleMobile,
}

type row struct {
  id           int64
  guid         string
  typ          int
  parent       int64
  position     int
  title        sql.NullString
  dateAdded    sql.NullInt64
  lastModified sql.NullInt64
  url          sql.NullString
}

// ReadProfile copies the profile's places.sqlite (plus WAL/SHM sidecars, if
// present) to a temp directory and parses it into the canonical model.
// Copying avoids "database is locked" failures while the browser is running.
// It does not set Source/Profile on the result; callers that know the
// product identity should set those.
func ReadProfile(profilePath string) (*model.Root, error) {
  tmpDir, err := os.MkdirTemp("", "bookmarks-firefox-*")
  if err != nil {
    return nil, fmt.Errorf("creating temp dir: %w", err)
  }
  defer os.RemoveAll(tmpDir)

  dbPath := filepath.Join(profilePath, "places.sqlite")
  tmpDB := filepath.Join(tmpDir, "places.sqlite")
  if err := copyFile(dbPath, tmpDB); err != nil {
    return nil, fmt.Errorf("copying places.sqlite: %w", err)
  }
  for _, suffix := range []string{"-wal", "-shm"} {
    src := dbPath + suffix
    if _, err := os.Stat(src); err == nil {
      if err := copyFile(src, tmpDB+suffix); err != nil {
        return nil, fmt.Errorf("copying places.sqlite%s: %w", suffix, err)
      }
    }
  }

  return readDB(tmpDB)
}

// Read parses the given profile for the given product, setting
// Source/Profile on the result.
func Read(product Product, profilePath string) (*model.Root, error) {
  root, err := ReadProfile(profilePath)
  if err != nil {
    return nil, err
  }
  root.Source = string(product)
  root.Profile = profilePath
  return root, nil
}

func readDB(dbPath string) (*model.Root, error) {
  db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
  if err != nil {
    return nil, fmt.Errorf("opening %q: %w", dbPath, err)
  }
  defer db.Close()

  rows, err := db.Query(`
    SELECT b.id, b.guid, b.type, b.parent, b.position, b.title,
           b.dateAdded, b.lastModified, p.url
    FROM moz_bookmarks b
    LEFT JOIN moz_places p ON b.fk = p.id
    ORDER BY b.parent, b.position
  `)
  if err != nil {
    return nil, fmt.Errorf("querying moz_bookmarks: %w", err)
  }
  defer rows.Close()

  var all []row
  nodes := map[int64]*model.Node{}
  var rootRowID *int64

  for rows.Next() {
    var r row
    if err := rows.Scan(&r.id, &r.guid, &r.typ, &r.parent, &r.position,
      &r.title, &r.dateAdded, &r.lastModified, &r.url); err != nil {
      return nil, fmt.Errorf("scanning row: %w", err)
    }
    all = append(all, r)

    n := &model.Node{
      ID:           r.guid,
      Title:        r.title.String,
      Index:        r.position,
      DateAdded:    r.dateAdded.Int64 / 1000,
      DateModified: r.lastModified.Int64 / 1000,
    }
    switch r.typ {
    case typeBookmark:
      n.Type = model.TypeBookmark
      n.URL = r.url.String
    case typeSeparator:
      n.Type = model.TypeSeparator
    default:
      n.Type = model.TypeFolder
    }
    nodes[r.id] = n

    if r.guid == rootGUID {
      id := r.id
      rootRowID = &id
    }
  }
  if err := rows.Err(); err != nil {
    return nil, err
  }
  if rootRowID == nil {
    return nil, fmt.Errorf("could not find root bookmark folder (guid %q)", rootGUID)
  }

  for _, r := range all {
    if r.id == *rootRowID {
      continue
    }
    parent, ok := nodes[r.parent]
    if !ok {
      continue
    }
    parent.Children = append(parent.Children, nodes[r.id])
  }

  root := &model.Root{Format: model.FormatFirefox}
  for _, child := range nodes[*rootRowID].Children {
    if internalGUIDs[child.ID] {
      continue
    }
    child.Role = rootRoles[child.ID]
    root.Roots = append(root.Roots, child)
  }
  return root, nil
}

func copyFile(src, dst string) error {
  in, err := os.Open(src)
  if err != nil {
    return err
  }
  defer in.Close()

  out, err := os.Create(dst)
  if err != nil {
    return err
  }
  defer out.Close()

  _, err = io.Copy(out, in)
  return err
}
