Bookmark transfer
=========

A command-line tool to move bookmarks between browsers. It reads a browser's native bookmark
storage - a Chromium `Bookmarks` JSON file, or a Firefox/LibreWolf `places.sqlite` database -
into a common JSON format, and converts that format into whatever a target browser can load.
Supports Firefox, LibreWolf, Chrome, Chromium, Brave, and Edge, plus arbitrary custom install or
profile paths.

# Install

```
go install github.com/DanSM-5/bookmarks-extractor/cmd/bookmarks@latest
```

This requires Go 1.25.6 or later. See [Building](#building) to build from source instead.

# Usage

```
bookmarks [command]
```

Run `bookmarks --help`, or `bookmarks <command> --help`, for the full built-in reference at any
time - the flag descriptions there always match the current build.

## Subcommands

### `extract`

Reads a browser's native bookmark storage and converts it into the common JSON format used by
`import`.

`--browser` accepts either a recognized name (`firefox`, `librewolf`, `chrome`, `chromium`,
`brave`, `edge`) or a path: a `Bookmarks` file, a `places.sqlite` file, a profile directory, or a
user-data/profiles root containing multiple profiles. The bookmark-store layout is auto-detected
from the path.

`--profile` accepts a directory name (e.g. `Default`, `Profile 1`), a display name shown in the
browser's own profile picker, or a Firefox `profiles.ini` profile name.
Run with `--list-profiles` first if you're not sure what's available.

### `import`

Converts the common JSON format produced by `extract` into whatever a target browser can
actually load, and explains how to load it.

By default this generates a Netscape-format HTML file and prints instructions for the browser's
own bookmark-import feature - a safe, non-destructive merge that adds a new folder alongside
whatever bookmarks are already there.

Pass `--replace` to instead overwrite the target's entire bookmark tree:

- Chromium-family targets (`chrome`/`chromium`/`brave`/`edge`): writes the `Bookmarks` file
  directly. The existing file is backed up first, and you'll be asked to confirm unless `--yes`
  is set.
- Firefox-family targets (`firefox`/`librewolf`): Firefox has no safe way to write its bookmark
  database directly (it relies on custom SQLite functions only Firefox's own process registers),
  so `--replace` instead generates a full-tree backup file for Firefox's own Restore feature -
  still a manual step, but one that preserves toolbar/menu/other placement, unlike the default
  HTML merge.

## Options and arguments

Flags shared by both subcommands:

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--browser` | | string | *(required)* | Browser name (`firefox`, `librewolf`, `chrome`, `chromium`, `brave`, `edge`) or a path to a custom install/profile location. |
| `--output` | `-o` | string | see below | Output file path. |
| `--dry-run` | | bool | `false` | Print what would happen without writing, backing up, or prompting. |
| `--help` | `-h` | bool | `false` | Show help for the command. |

`extract`-only:

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--profile` | | string | browser default | Profile to read from: directory name, display name, or `profiles.ini` name. |
| `--list-profiles` | | bool | `false` | List available profiles for `--browser` and exit. |

For `extract`, `--output` defaults to stdout.

`import`-only:

| Flag | Short | Type | Default | Description |
| --- | --- | --- | --- | --- |
| `--input` | | string | *(required)* | Common-format bookmarks JSON file to import. |
| `--profile` | | string | `Default` | Target profile: directory name or display name. Only consulted when `--replace` targets a chromium-family browser - ignored otherwise, since the default merge flow doesn't pick a profile for you (you choose it yourself in the browser's own import dialog), and Firefox-family targets never take a profile either way. |
| `--replace` | | bool | `false` | Overwrite the target's bookmarks directly instead of generating a file to merge in manually. |
| `--yes` | `-y` | bool | `false` | Skip the confirmation prompt when `--replace` targets a chromium-family browser. |

For `import`, `--output` is where the generated merge/restore file is written and defaults to a
path next to `--input`.

Top-level (on `bookmarks` itself):

| Flag | Short | Type | Description |
| --- | --- | --- | --- |
| `--help` | `-h` | bool | Show help and exit. |
| `--version` | `-v` | bool | Print `<version>@<commit>` and exit. |

## Samples

Extract Chrome's default profile to a file:

```
bookmarks extract --browser chrome -o bookmarks.json
```

Extract a specific Firefox profile by its `profiles.ini` name:

```
bookmarks extract --browser firefox --profile default-release -o bookmarks.json
```

Extract using a profile's display name instead of its directory name (Chromium-family browsers
only):

```
bookmarks extract --browser chrome --profile "foo" -o bookmarks.json
```

List the profiles available for a browser before picking one:

```
bookmarks extract --browser chrome --list-profiles
```

Preview an extraction - shows the resolved profile, the exact file that would be read, a
bookmark count/preview, and the output path - without writing anything:

```
bookmarks extract --browser firefox --profile default-release --dry-run
```

Extract directly from a custom path instead of a recognized browser name (a `Bookmarks` file, a
`places.sqlite` file, a profile directory, or a user-data root are all accepted):

```
bookmarks extract --browser /path/to/custom/profile -o bookmarks.json
```

Import into Chrome as a safe, non-destructive merge - generates an HTML file and prints
instructions for Chrome's own "Import bookmarks" feature (`--profile` isn't needed here: you pick
the target profile yourself in Chrome's own import dialog):

```
bookmarks import --browser chrome --input bookmarks.json
```

Import into Firefox the same non-destructive way - generates an HTML file for Firefox's "Import
Bookmarks from HTML File" feature (note: this always lands everything under Bookmarks Menu,
regardless of where it was originally):

```
bookmarks import --browser firefox --input bookmarks.json
```

Overwrite a Chromium-family profile's bookmarks directly, skipping the confirmation prompt (a
backup of the existing file is still made automatically):

```
bookmarks import --browser edge --profile Default --input bookmarks.json --replace --yes
```

Replace Firefox's entire bookmark tree instead of merging - generates a backup file for Firefox's
own Restore feature, which (unlike the HTML merge) preserves toolbar/menu/other placement:

```
bookmarks import --browser firefox --input bookmarks.json --replace
```

Preview a `--replace` import - shows the resolved target, any conversion warnings (dropped
separators, regenerated IDs), and a bookmark summary, without touching anything:

```
bookmarks import --browser chrome --profile test --input bookmarks.json --replace --dry-run
```

Full round trip - move bookmarks from a Chrome profile into Brave:

```
bookmarks extract --browser chrome --profile Default -o bookmarks.json
bookmarks import --browser brave --profile Default --input bookmarks.json --replace --yes
```

# Structure

```
.
├── cmd/
│   └── bookmarks/          # CLI entrypoint (Cobra commands)
│       ├── main.go         # root command, wiring
│       ├── extract.go      # `extract` subcommand
│       ├── import.go       # `import` subcommand
│       ├── paths.go        # shared browser-path auto-detection
│       ├── summary.go      # bookmark summary/preview printing (--dry-run)
│       ├── version.go      # version string embedding
│       └── version.txt     # semver, embedded at compile time
├── internal/
│   ├── model/              # canonical bookmark tree format shared by every reader/writer
│   │   └── node.go
│   ├── chromium/           # Chrome/Chromium/Brave/Edge: reads and writes the Bookmarks JSON
│   │   ├── paths.go        # user-data-dir discovery, profile resolution
│   │   ├── reader.go
│   │   └── writer.go
│   ├── firefox/            # Firefox/LibreWolf: reads places.sqlite, writes a backup JSON
│   │   ├── paths.go        # profiles.ini discovery, profile resolution
│   │   ├── reader.go
│   │   └── backup.go       # Firefox-native bookmark backup JSON writer (for --replace)
│   └── netscape/           # Netscape Bookmark HTML writer (the default, merge-style import)
│       └── writer.go
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── .gitignore
```

# Building

1. Install Go 1.25.6 or later.
2. Clone the repository:
   ```
   git clone https://github.com/DanSM-5/bookmarks-extractor.git
   cd bookmarks-extractor
   ```
3. Build the binary:
   ```
   go build -o bookmarks ./cmd/bookmarks
   ```
   On Windows, use an `.exe` output name instead:
   ```
   go build -o bookmarks.exe ./cmd/bookmarks
   ```
4. Verify it works:
   ```
   ./bookmarks --version
   ```
5. Optionally, install it onto your `PATH` (into `$(go env GOPATH)/bin`):
   ```
   go install ./cmd/bookmarks
   ```

No build tags, code generation, or external tools (`make`, etc.) are required - a plain `go
build` is sufficient. The version string embeds the git commit automatically via
`runtime/debug.ReadBuildInfo()` when building from a git checkout; see `cmd/bookmarks/version.go`.

# Dependencies

- **Go 1.25.6** or later (see `go.mod`)

Direct dependencies:

| Module | Version | Used for |
| --- | --- | --- |
| [`github.com/spf13/cobra`](https://github.com/spf13/cobra) | v1.10.2 | CLI framework: subcommands, flags, and help/usage generation. |
| [`github.com/google/uuid`](https://github.com/google/uuid) | v1.6.0 | Generating and validating GUIDs when converting bookmark IDs between browsers with incompatible ID formats. |
| [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) | v1.53.0 | Pure-Go (no cgo) SQLite driver, used to read Firefox's `places.sqlite`. |

Indirect (transitive) dependencies, pulled in by the above:

| Module | Version |
| --- | --- |
| `github.com/dustin/go-humanize` | v1.0.1 |
| `github.com/inconshreveable/mousetrap` | v1.1.0 |
| `github.com/mattn/go-isatty` | v0.0.20 |
| `github.com/ncruces/go-strftime` | v1.0.0 |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec |
| `github.com/spf13/pflag` | v1.0.9 |
| `golang.org/x/sys` | v0.44.0 |
| `modernc.org/libc` | v1.73.4 |
| `modernc.org/mathutil` | v1.7.1 |
| `modernc.org/memory` | v1.11.0 |

# License

MIT - see [LICENSE](LICENSE).
