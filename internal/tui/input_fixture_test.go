package tui

import tea "charm.land/bubbletea/v2"

func press(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	default:
		r := []rune(s)
		return tea.KeyPressMsg{Text: s, Code: r[0]}
	}
}
