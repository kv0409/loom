---
inclusion: always
---

# Bubbles Best Practices

The dashboard uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) with [Bubbles](https://github.com/charmbracelet/bubbles) components and [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling.

## Rule: Use Native Bubbles Components

Never hand-roll functionality that a Bubbles component already provides. The library handles ANSI-aware width calculation, unicode edge cases, terminal compatibility, and accessibility — hand-rolled equivalents will have bugs.

| Need | Use | Never hand-roll |
|------|-----|-----------------|
| Columnar data | `bubbles/table` via `newStyledTable()` in `render_helpers.go` | `fmt.Sprintf("%-*s", ...)`, manual `lipgloss.Width()` padding, `strings.Repeat(" ", ...)` for column alignment |
| Text input | `bubbles/textinput` | Rune-level keyboard handling, manual cursor tracking, character insertion/deletion |
| Scrollable content | `bubbles/viewport` | Manual scroll offset math, hand-rolled `listViewport()` functions |
| Loading/activity indicator | `bubbles/spinner` | Custom frame-cycling animation |
| Keybindings | `bubbles/key` | Raw `tea.KeyMsg` string matching |
| Help bar | `bubbles/help` | Manual help string construction |

## All Columnar Rendering Goes Through `newStyledTable()`

Every view that renders columns — whether it has headers or not — must use `newStyledTable()` from `render_helpers.go`. This is the single interface for all tabular/columnar content in the dashboard.

- Full table views (agents list, issues list, mail, memory, activity, diff): use `newStyledTable()` with headers.
- Compact panel sections (overview agent band, overview activity): use `newStyledTable()` configured headerless if needed.
- No view should calculate column widths or pad cells manually. If `newStyledTable()` doesn't support a layout you need, extend it — don't bypass it.

## Styling Through `styles.go`

All lipgloss styles live in `internal/dashboard/styles.go`. Never create inline `lipgloss.NewStyle()` calls in view files.

- Define named styles in `styles.go` and reference them in views.
- Agent colors use `agentPill(id)` for background-filled badges and `agentColor(id)` for the raw color value.
- Status rendering uses `statusPill(status)` — fixed-width, background-filled.

## Adding New Bubbles Components

When introducing a new Bubbles component:

1. Initialize it in the dashboard `Model` struct (not created per-frame).
2. Wire its `Update()` into the dashboard's `Update()` method.
3. Call its `View()` in the appropriate render function.
4. Style it consistently with the Tokyo Night palette from `styles.go`.
