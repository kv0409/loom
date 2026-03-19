package dashboard

import (
	"image/color"
)

// toolInfo maps a tool label to a display icon and color.
type toolInfo struct {
	icon  string
	color color.Color
}

// toolMap maps uppercase tool labels (as produced by backend.parseToolFields)
// to a single-cell icon and color.
var toolMap = map[string]toolInfo{
	"SHELL": {"❯", colCyan},
	"READ":  {"←", colGreen},
	"WRITE": {"✎", colYellow},
	"FIND":  {"⌕", colCyan},
	"CODE":  {"◆", colBlue},
	"AWS":   {"☁", colOrange},
	"LOOM":  {"⚙", colMagenta},
}

// resolveToolInfo returns the toolInfo for a given tool label.
func resolveToolInfo(label string) toolInfo {
	if info, ok := toolMap[label]; ok {
		return info
	}
	return toolInfo{"·", colGray}
}
