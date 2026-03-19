---
inclusion: always
---

# Bubbles Best Practices

The dashboard uses [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) (`charm.land/bubbletea/v2`) with [Bubbles v2](https://github.com/charmbracelet/bubbles) (`charm.land/bubbles/v2`) components and [Lip Gloss v2](https://github.com/charmbracelet/lipgloss) (`charm.land/lipgloss/v2`) for styling.

## Rule: Use Native Bubbles Components

Never hand-roll functionality that a Bubbles component already provides. The library handles ANSI-aware width calculation, unicode edge cases, terminal compatibility, and accessibility — hand-rolled equivalents will have bugs.

| Need | Use | Never hand-roll |
|------|-----|-----------------|
| Columnar data | `lipgloss/table` via `newLGTable()` in `render_helpers.go` | `fmt.Sprintf("%-*s", ...)`, manual `lipgloss.Width()` padding, `strings.Repeat(" ", ...)` for column alignment |
| Text input | `bubbles/textinput` | Rune-level keyboard handling, manual cursor tracking, character insertion/deletion |
| Multi-line text editing | `bubbles/textarea` | Custom newline/cursor handling |
| Scrollable content | `bubbles/viewport` via `detailVP`/`diffVP` in `Model` | Manual slice indexing, `strings.Split` + range loops with offset |
| Progress bar | `bubbles/progress` | Custom frame-cycling animation |
| Loading/activity indicator | `bubbles/spinner` via `spinner` in `Model` | Custom frame-cycling animation |
| Keybindings display | `bubbles/key` + `bubbles/help` | Manual help string construction |

## All Columnar Rendering Goes Through `newLGTable()`

Every view that renders columns — whether it has headers or not — must use `newLGTable()` or `newLGTableHeaderless()` from `render_helpers.go`. These are the single interface for all tabular/columnar content in the dashboard, backed by `lipgloss/table`.

- Full table views (agents list, issues list, mail, memory, activity, diff): use `newLGTable()` with headers.
- Compact panel sections (overview agent band, overview activity): use `newLGTableHeaderless()`.
- No view should calculate column widths or pad cells manually. If `newLGTable()` doesn't support a layout you need, extend it — don't bypass it.

**Tables are rebuilt every frame.** `lipgloss/table` is a static renderer — it has no `Update()` method and is not stored in `Model`. The selected row index is passed to `newLGTable()` which applies highlight styling via `StyleFunc`:

```go
start, end := listViewport(m.cursor, len(items), vRows)
rows := buildRows(items[start:end])
t := newLGTable(headers, rows, m.cursor-start, availableWidth(m.width))
return t.Render()
```

This is intentional — it keeps state flat. Do not store table instances in `Model`.

## Styling Through `styles.go`

All lipgloss styles live in `internal/dashboard/styles.go`. Never create inline `lipgloss.NewStyle()` calls in view files.

- Define named styles in `styles.go` and reference them in views.
- Agent colors use `agentPill(id)` for background-filled badges and `agentColor(id)` for the raw color value.
- Status rendering uses `statusPill(status)` — fixed-width, background-filled.
- **All bordered panel containers use `panel(title, content, width)`** — the single wrapper for every boxed section. Never construct border styles inline.
- Horizontal dividers inside panels use `separator(w)` — never `strings.Repeat("─", ...)` inline.

## Layout Helpers from `layout.go`

`layout.go` provides the single source of truth for all size calculations. **Never compute `m.width - N` or `m.height - N` inline in view files.** Call a helper, or add one to `layout.go` if none fits.

| Helper | Returns | Use for |
|--------|---------|---------|
| `availableWidth(m.width)` | `w - 6`, min 40 | Usable column width inside a panel |
| `panelWidth(m.width)` | `w - 2` | `width` arg passed to `panel()` |
| `visibleRows(m.height, 9)` | `h - headerRows`, min 1 | Scrollable row budget for tab views (9 fixed header rows) |
| `scrollViewport(m.height)` | `h - 6`, min 1 | Scroll height for detail/panel views |
| `proportionalWidth(avail, pct, minW)` | `max(minW, avail*pct/100)` | Column width as a percentage of available space |
| `listViewport(cursor, total, vRows)` | `(start, end)` | Cursor-following window into a list slice |
| `separator(w)` | `"  ───…"` line | Horizontal rule inside a panel |

```go
avail        := availableWidth(m.width)
vRows        := visibleRows(m.height, 9)
viewH        := scrollViewport(m.height)
start, end   := listViewport(m.cursor, len(items), vRows)
colW         := proportionalWidth(avail, 40, 10)  // 40% of avail, min 10
```

For multi-panel layouts, join rendered panel strings:

```go
// Side-by-side panels aligned at the top:
lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

// Stacked panels aligned to the left:
lipgloss.JoinVertical(lipgloss.Left, topPanel, bottomPanel)
```

## Scrollable Detail Views

The dashboard uses `bubbles/viewport` for all detail views. Two viewport instances live in `Model`:
- `detailVP` — for agent detail, issue detail, memory detail, mail detail
- `diffVP` — for diff view (horizontal scrolling enabled via `SetHorizontalStep`)

Scroll position is tracked via offset fields (`detailYOff`, `diffYOff`, `diffXOff`) because the viewport requires content to be set before scroll methods work, and content is only available in the render functions (View has a value receiver).

**Pattern for a new scrollable view:**

```go
// 1. In the render function, build lines and set on a viewport copy:
lines := buildMyDetailLines(m)
vp := m.detailVP                    // copy (View is value receiver)
vp.SetContentLines(lines)
vp.SetYOffset(m.detailYOff)         // apply tracked scroll position
scrollInfo := vpScrollIndicator(vp) // "↑3 ↓7" or ""
return panel("Title"+scrollInfo, vp.View(), panelWidth(m.width))

// 2. Handle scroll keys in handleKey():
case keyVimDown, keyDown:
    m.detailYOff++
    return m, nil

// 3. Reset scroll on view entry (in handleEnter):
m.detailYOff = 0
```

The diff view uses `StyleLineFunc` for per-line coloring (e.g., green for additions, red for deletions) and native horizontal scrolling.

## Key Bindings

Key handling uses a three-part pattern. All three parts must be present for a new key.

**1. Add a constant to `keys.go`** (the canonical string, one place):

```go
// In the appropriate const block in keys.go:
keyMyAction = "r"
```

**2. Add a `key.Binding` to `keyMap` in `keys.go`** (for the help bar display — not for dispatch):

```go
// In the keyMap struct:
MyAction key.Binding

// In defaultKeyMap():
MyAction: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

// In ShortHelp() or FullHelp():
func (k keyMap) ShortHelp() []key.Binding { return []key.Binding{..., k.MyAction} }
```

**3. Dispatch via raw string `case` in `handleKey()`** — never use `key.Matches()`. In v2, key events arrive as `tea.KeyPressMsg` (not `tea.KeyMsg`), but the `msg.String()` dispatch pattern is unchanged:

```go
case keyMyAction:
    // handle it
```

`key.Binding` values exist solely to power the `help.Model` display. All actual routing goes through the string constants.

## Async Commands and the Tick Loop

`tea.Cmd` is a `func() tea.Msg` — it runs in a goroutine and delivers its result back to `Update()`.

```go
// Commands that need context use a closure:
func fetchThingCmd(id string) tea.Cmd {
    return func() tea.Msg {
        result, err := doWork(id)
        if err != nil {
            return errMsg{err}
        }
        return thingMsg{result}
    }
}

// Handle in Update():
case thingMsg:
    m.data.thing = msg.result
    return m, nil
```

**To add new data to the refresh cycle:** extend the `data` struct and the `refresh()` command in `app.go`. **Do not add a separate ticker.** The existing `tickMsg` loop fires every 2 seconds and loads all domain data in one atomic pass — partial/concurrent loads cause flicker and inconsistent views.

**To fan out multiple commands at once** (e.g., in `Init()`):

```go
return m, tea.Batch(cmd1, cmd2, cmd3)
```

## Terminal Sizing

The dashboard stores terminal dimensions from `tea.WindowSizeMsg` and derives all layout from them.

```go
// In Update():
case tea.WindowSizeMsg:
    m.width, m.height = msg.Width, msg.Height
    m.help.SetWidth(msg.Width)   // propagate to help component
    return m, nil                // no Cmd needed
```

- All sizing in `View()` and render functions must derive from `m.width` and `m.height`.
- **Never hardcode widths or heights.**
- The minimum supported terminal size is 60×15. `View()` renders a centered error message below this threshold — no layout logic runs.
- If a new Bubbles component has a `SetWidth` / `SetHeight` method, call it in the `WindowSizeMsg` handler.
- Bubbles v2 uses getter/setter methods instead of direct field access for Width/Height on help, table, textinput, viewport, etc. (e.g., `m.help.SetWidth(w)` not `m.help.Width = w`).

## Adding New Bubbles Components

When introducing a new stateful Bubbles component (textinput, spinner, progress, etc.):

1. Initialize it in the dashboard `Model` struct (not created per-frame).
2. Wire its `Update()` into the dashboard's `Update()` method inside the appropriate modal guard or unconditionally.
3. Call its `View()` in the appropriate render function.
4. Style it consistently with the Tokyo Night palette from `styles.go`.
5. If it has a width/height setter, sync it in the `tea.WindowSizeMsg` handler.

**Exception — tables:** do not follow this guide for tables. Tables are rebuilt per frame via `newLGTable()` / `newLGTableHeaderless()`. See the columnar rendering rule above.

## View Return Type

The dashboard's `View()` method returns `tea.View` (not `string`). Terminal features are set declaratively as View fields:

```go
func (m Model) View() tea.View {
    // ... build content string ...
    v := tea.NewView(content)
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion
    return v
}
```

Child components and render functions still return `string` — only the top-level `View()` returns `tea.View`. The `tea.View` struct has fields for: `Content`, `AltScreen`, `MouseMode`, `ReportFocus`, `WindowTitle`, `Cursor`, `BackgroundColor`, `ForegroundColor`, `ProgressBar`, `KeyboardEnhancements`.

## Mouse Messages

Mouse events are split into separate types in v2:
- `tea.MouseClickMsg` — button clicks (`.Button`, `.X`, `.Y`)
- `tea.MouseWheelMsg` — scroll events (`.Button` is `tea.MouseWheelUp`/`tea.MouseWheelDown`, `.X`, `.Y`)
- `tea.MouseMotionMsg` — movement events
- `tea.MouseReleaseMsg` — button releases

Button constants: `tea.MouseLeft`, `tea.MouseRight`, `tea.MouseMiddle`, `tea.MouseWheelUp`, `tea.MouseWheelDown`.

The dashboard handles clicks in `handleMouseClick()` and wheel events in `handleMouseWheel()`.

## New in v2 (Available but not yet adopted)

- **Progressive keyboard enhancements**: `shift+enter`, `ctrl+h`, key releases. Set `view.KeyboardEnhancements.ReportEventTypes = true` for key release events. Listen for `tea.KeyboardEnhancementsMsg` to detect support.
- **Native clipboard**: `tea.SetClipboard(text)`, `tea.ReadClipboard()` — works over SSH via OSC52
- **Cursor control**: `view.Cursor` — position, shape (block/bar/underline), color, blink
- **Terminal color queries**: `tea.RequestBackgroundColor`, `tea.RequestForegroundColor` — detect light/dark theme
- **Progress bar**: `view.ProgressBar` — native terminal progress indicator
- **Environment variables**: `tea.EnvMsg` — get client environment (important for SSH/Wish apps)
- **`tea.WithColorProfile()` and `tea.WithWindowSize()`**: Program options for testing without a real terminal

## v2 Reference

For API details, use `go doc` from the project root:
- `go doc charm.land/bubbletea/v2` — Bubble Tea package
- `go doc charm.land/bubbletea/v2.View` — View struct fields
- `go doc charm.land/bubbletea/v2.KeyPressMsg` — Key press message
- `go doc charm.land/bubbletea/v2.MouseClickMsg` — Mouse click message
- `go doc charm.land/bubbles/v2/help` — Help component
- `go doc charm.land/bubbles/v2/key` — Key binding

Upstream docs:
- [Bubble Tea Upgrade Guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md)
- [Bubble Tea What's New](https://github.com/charmbracelet/bubbletea/discussions/1374)
- [Bubbles Upgrade Guide](https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md)
- [Bubble Tea API](https://pkg.go.dev/charm.land/bubbletea/v2)
- [Bubbles API](https://pkg.go.dev/charm.land/bubbles/v2)
