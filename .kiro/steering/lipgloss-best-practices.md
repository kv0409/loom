---
inclusion: always
---

# Lip Gloss Best Practices

The dashboard uses **Lip Gloss v2** (`charm.land/lipgloss/v2`).

All project lipgloss code lives in `internal/dashboard/styles.go`. View and render files are consumers only — they call helpers from `styles.go`, never construct styles themselves.

## Color Palette

All colors are defined as package-level `var` at the top of `styles.go` using the **Tokyo Night** truecolor palette. In v2, `lipgloss.Color()` is a function returning `color.Color` (from `image/color`), not a distinct type. The call syntax `lipgloss.Color("#1A1B26")` is unchanged, but maps or function return types that previously used `lipgloss.Color` as a type must use `color.Color` instead (e.g., `statusColors map[string]color.Color`, `agentColor() color.Color`):

```go
var (
    colBg      = lipgloss.Color("#1A1B26") // background
    colBlue    = lipgloss.Color("#7AA2F7") // primary / selected
    colGreen   = lipgloss.Color("#9ECE6A") // success / active
    colYellow  = lipgloss.Color("#E0AF68") // warning
    colRed     = lipgloss.Color("#F7768E") // error / blocked
    colCyan    = lipgloss.Color("#7DCFFF") // review / info
    colMagenta = lipgloss.Color("#BB9AF7") // lead
    colOrange  = lipgloss.Color("#FF9E64") // dead / orchestrator
    colTeal    = lipgloss.Color("#73DACA") // in-progress / explorer
    colGray    = lipgloss.Color("#565F89") // idle / subtle text
    colFg      = lipgloss.Color("#C0CAF5") // default foreground
    colSubtle  = lipgloss.Color("#414868") // borders, dim ui
    colSelBg   = lipgloss.Color("#292E42") // selection background
)
```

**Rules:**
- **Never** write `lipgloss.Color("#...")` in view files or render helpers. Add to the palette in `styles.go` if a new color is genuinely needed.
- Prefer semantic assignments: map colors to roles (`colRed` for errors, `colGreen` for success) rather than using raw hex values in style definitions.
- Lip Gloss automatically downgrades truecolor hex values to ANSI 256 or 16-color in lesser terminals. No special handling needed for terminal compatibility.

## Where Styles Live: Static vs Dynamic

**Static styles** (identical every call) → package-level `var` in `styles.go`:

```go
var titleStyle = lipgloss.NewStyle().Bold(true).Background(colBlue).Foreground(colBg).Padding(0, 2)
var helpStyle  = lipgloss.NewStyle().Foreground(colSubtle)
```

**Dynamic styles** (vary by input parameter) → functions in `styles.go` returning `lipgloss.Style` or `string`:

```go
// Returns a Style — caller can chain or render:
func statusStyle(status string) lipgloss.Style {
    if c, ok := statusColors[status]; ok {
        return lipgloss.NewStyle().Foreground(c)
    }
    return idleStyle
}

// Returns a pre-rendered string — for single-use badges:
func agentPill(id string) string {
    return lipgloss.NewStyle().
        Background(agentColor(id)).
        Foreground(colBg).
        Bold(true).
        Padding(0, 1).
        Render(id)
}
```

**Rule:** Never write `lipgloss.NewStyle()` in view files (`agents.go`, `issues.go`, etc.) or `render_helpers.go`. If a style is needed in a view, it either already exists in `styles.go` or must be added there.

## Style is a Value Type

`lipgloss.Style` is a struct of primitives. **Assignment creates a copy.** Method calls return a new value without mutating the receiver:

```go
base := lipgloss.NewStyle().Foreground(colFg)
bold := base.Bold(true)    // new value; base is unchanged
red  := base.Foreground(colRed) // another new value from the original base

// Correct per-call dynamic styling:
func heartbeatStyle(ago string) lipgloss.Style {
    if strings.HasSuffix(ago, "s") {
        return lipgloss.NewStyle().Foreground(colGreen) // fresh value each call
    }
    ...
}
```

This means it is **safe to store styles as package-level `var`** — concurrent reads are safe, and callers chaining additional methods do not corrupt the base.

## Width, Sizing, and the Border Frame

### Always measure styled strings with `lipgloss.Width()`

`len(s)` counts **bytes**. `lipgloss.Width(s)` returns the **terminal cell width**, correctly handling ANSI escape codes, multi-byte unicode, and double-width CJK characters:

```go
// Wrong — counts bytes including hidden ANSI codes:
if len(styledStr) > maxW { ... }

// Correct — counts visible cells:
if lipgloss.Width(styledStr) > maxW { ... }
```

Use `lipgloss.Width()` any time you measure a string that may contain styles or non-ASCII characters. The `panel()` and `truncate()` functions in `styles.go` do this correctly. Note: `truncate()` uses `ansi.Truncate()` from `charmbracelet/x/ansi` internally, which is ANSI-escape-aware.

### `.Width()` is a minimum, not a cap

`.Width(n)` sets the **minimum** rendered width (padding included). Content wider than `n` extends the block beyond `n`. To **cap** width, use `.MaxWidth(n)` or the project's `truncate(s, n)`:

```go
// Sets minimum width — may exceed n if content is long:
someStyle.Width(n).Render(longString)

// Caps to exactly n cells (with "..." suffix if truncated):
truncate(longString, n)

// Inline max-width clamp without "...":
lipgloss.NewStyle().MaxWidth(n).Render(longString)
// This is what truncateLines() uses internally.
```

### Border frame overhead

A style with `Border(RoundedBorder())` adds **2 cells** to each axis (1 left + 1 right, 1 top + 1 bottom). You must account for this when computing the content width to pass to `.Width()`:

```go
// panel() in styles.go does this correctly:
innerW := panelWidth(m.width) - 2   // panelWidth = m.width - 2; minus 2 more for border
borderStyle.Width(innerW).Render(content)
// Result: outer width = innerW + 2 (borders) = m.width - 2
```

The `borderStyle` variable uses `RoundedBorder()`. Always use it via `panel()` — never construct bordered styles inline.

### Padding and width interact

`.Padding(0, 1)` adds 1 cell on each side. When combined with `.Width(n)`, the padding is **included** in the total width — content gets `n - 2` usable cells:

```go
// statusPill has Width(13) and Padding(0,1):
// → 13 total cells = 1 left pad + 11 content + 1 right pad
lipgloss.NewStyle().Width(13).Padding(0, 1).Render(status)
```

## Text Alignment

Alignment only takes effect when a width is set:

```go
// Centers text in 80 cells:
lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(text)

// renderEmpty() in styles.go does this:
centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
return centered.Render(emptyMsgStyle.Render(msg)) + "\n"
```

Available positions: `lipgloss.Left` (default), `lipgloss.Center`, `lipgloss.Right`.

## Layout Composition

### Joining panels

```go
// Side by side, tops aligned — used for kanban columns, overview bands:
lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel, ...)

// Stacked, left-aligned — used for overview view stacking panels:
lipgloss.JoinVertical(lipgloss.Left, topPanel, bottomPanel, ...)
```

The argument to `JoinHorizontal`/`JoinVertical` is the alignment of the shorter blocks against the longer:
- `lipgloss.Top` — shorter panels align to top (most common for dashboards)
- `lipgloss.Center` — center-aligned
- `lipgloss.Bottom` — shorter panels align to bottom
- A float `0.0–1.0` — proportional position

### Placing text in whitespace

For centering a rendered block within a fixed space (e.g., the minimum-size error message):

```go
// Center a block horizontally in the full terminal width:
lipgloss.PlaceHorizontal(m.width, lipgloss.Center, block)

// Center a block in both dimensions:
lipgloss.Place(m.height, m.width, lipgloss.Center, lipgloss.Center, block)
```

### The `panel()` function — all bordered containers

Every bordered panel in the dashboard goes through `panel(title, content, width)` in `styles.go`. It:
1. Truncates `content` lines to `innerW = width - 2` cells (preventing overflow past the border)
2. Renders with `borderStyle.Width(innerW)` (rounded border, subtle foreground)
3. Replaces the top border line with a custom `╭─ [TITLE] ───╮` bar when `title != ""`

```go
// Correct — all bordered panels:
return panel("AGENTS", tableContent, panelWidth(m.width))

// Wrong — bypasses truncation, uses inconsistent border style:
return borderStyle.Width(m.width - 4).Render(tableContent)
```

The `width` argument to `panel()` is always `panelWidth(m.width)` (= `m.width - 2`). The `-2` accounts for the panel being used inside the full-width view which already has 1-cell insets on each side.

## Rendering Snippets

### Fixed-width pill badges

Pattern: background fill, dark foreground, `Padding(0, 1)` for visual breathing room, fixed `Width()` for column alignment:

```go
// Variable-width pill (content determines width):
lipgloss.NewStyle().Background(c).Foreground(colBg).Bold(true).Padding(0, 1).Render(label)

// Fixed-width pill (column-aligned across rows):
lipgloss.NewStyle().Background(c).Foreground(colBg).Bold(true).Padding(0, 1).Width(13).Render(label)
```

### Per-line diff coloring

For text content where each line needs independent styling (diff views, log lines), apply style per line rather than to the entire block:

```go
for _, line := range lines {
    switch {
    case strings.HasPrefix(line, "+"):
        out = append(out, diffAdd.Render(line))
    case strings.HasPrefix(line, "-"):
        out = append(out, diffDel.Render(line))
    default:
        out = append(out, line)
    }
}
content = strings.Join(out, "\n")
```

Do not apply a block-level style to multi-line diff content — it re-flows lines.

### Inline styles for glyphs/indicators

Single-character glyphs and status indicators use one-time inline styles (these are not stored as vars because they depend on state):

```go
func statusIndicator(status string) string {
    glyph := statusGlyphs[status]
    return statusStyle(status).Render(glyph)  // statusStyle returns lipgloss.Style
}
```

## Common Pitfalls

| Mistake | Correct pattern |
|---------|----------------|
| `len(styledStr) > n` | `lipgloss.Width(styledStr) > n` |
| `someStyle.Width(n)` to truncate long content | `truncate(s, n)` or `.MaxWidth(n)` |
| `borderStyle.Width(m.width)` — forgetting border overhead | `borderStyle.Width(m.width - 2)` or use `panel()` |
| `strings.Split(s, "\n")` on styled multiline content | `splitLines(s)` from `styles.go` (handles edge cases) |
| `lipgloss.NewStyle()` in view files | Add style to `styles.go` first |
| `s.Bold(true); use s` — expecting mutation | `s = s.Bold(true)` — always reassign |
| `style.Width(n).Border(...)` on full terminal width | Compute `innerW = w - 2` first, pass to `.Width(innerW)` |
| `lipgloss.JoinHorizontal(lipgloss.Left, ...)` — wrong alignment | `JoinHorizontal(lipgloss.Top, ...)` for side-by-side panels |

## Testing

Lip Gloss strips all ANSI codes when output is not a TTY, which includes unit tests. If a test asserts on styled output (e.g., checking for color or bold), force the color profile via `tea.WithColorProfile` when creating the program:

```go
import (
    "github.com/charmbracelet/colorprofile"
    tea "charm.land/bubbletea/v2"
)

// In test setup, use tea.WithColorProfile:
p := tea.NewProgram(model, tea.WithColorProfile(colorprofile.TrueColor))
```

For standalone lipgloss testing outside Bubble Tea, color downsampling is handled at the output layer — the `muesli/termenv` import is no longer needed.

Note: Our current tests don't use `SetColorProfile`, so this is informational.

Most dashboard tests avoid asserting on ANSI codes and instead test plain-text content or `lipgloss.Width()` of rendered output. Prefer this approach — it avoids fragile ANSI string comparisons.

## New in v2 (Available but not yet adopted)

These features are available in Lip Gloss v2 but not yet used in this project. Consider adopting them when the use case arises:

- **Hyperlinks**: `style.Hyperlink(url)` — clickable links in supporting terminals
- **Curly underlines**: `style.UnderlineStyle(lipgloss.UnderlineCurly)` with `style.UnderlineColor(c)` — useful for error indicators
- **Border gradient blending**: `style.BorderForegroundBlend(colors...)` — gradient borders
- **`lipgloss/list` sub-package**: Structured list rendering — could replace hand-built bullet lists in detail views
- **Named ANSI colors**: `lipgloss.Red`, `lipgloss.Green`, etc. — 16 basic ANSI color constants
- **`lipgloss.Println/Printf/Sprint`**: For standalone (non-Bubble Tea) output with automatic color downsampling. Use in `internal/cli/output.go` instead of `fmt.Println` for proper terminal color handling.

## v2 Reference

For API details beyond what's covered here, use `go doc` from the project root (requires v2 deps in go.mod):
- `go doc charm.land/lipgloss/v2` — package overview
- `go doc charm.land/lipgloss/v2.Style` — Style type and methods
- `go doc charm.land/lipgloss/v2.Color` — Color function

Upstream docs:
- [Upgrade Guide](https://github.com/charmbracelet/lipgloss/blob/main/UPGRADE_GUIDE_V2.md)
- [What's New](https://github.com/charmbracelet/lipgloss/discussions/506)
- [API Reference](https://pkg.go.dev/charm.land/lipgloss/v2)
