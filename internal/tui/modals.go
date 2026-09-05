package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	supportEmail     = "support@termon.sh"
	supportIssuesURL = "https://github.com/somus/termon/issues"
)

func (m Model) handleInfoModalInput(msg tea.Msg) (Model, tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil, false
	}
	if m.modal != modalNone {
		switch key.String() {
		case "esc", "q", "S", "?":
			m.modal = modalNone
		}
		return m, nil, true
	}
	if m.screen != screenLobby || m.save == nil {
		return m, nil, false
	}
	switch key.String() {
	case "S":
		m.modal = modalStats
		m.wb.trainerStats = nil
		m.wb.worldStats = nil
		m.wb.statsError = ""
		return m, m.loadTrainerStatsCmd(), true
	case "?":
		m.modal = modalHelp
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) renderInfoModal(kind modalKind) string {
	switch kind {
	case modalStats:
		return m.renderStatsModal()
	case modalHelp:
		return m.renderHelpModal()
	default:
		return ""
	}
}

func (m Model) renderStatsModal() string {
	lines := []string{titleStyle.Render("TRAINER STATS"), ""}
	if m.wb.statsError != "" {
		lines = append(lines,
			warnStyle.Render("Statistics unavailable"),
			dimStyle.Render(m.wb.statsError),
		)
	} else if stats := m.wb.trainerStats; stats != nil {
		matches := stats.Wins + stats.Losses
		winRate := 0.0
		if matches > 0 {
			winRate = float64(stats.Wins) * 100 / float64(matches)
		}
		streak := strconv.Itoa(stats.CurrentStreak)
		if stats.CurrentStreak > 0 {
			streak = fmt.Sprintf("W%d", stats.CurrentStreak)
		} else if stats.CurrentStreak < 0 {
			streak = fmt.Sprintf("L%d", -stats.CurrentStreak)
		}
		lines = append(lines,
			selStyle.Render("SUPPORT ID  ")+groupedSupportID(stats.TrainerID),
			"Trainer since  "+stats.CreatedAt.UTC().Format("2006-01-02"),
			"",
			fmt.Sprintf("Battles        %d", matches),
			fmt.Sprintf("Record         %dW / %dL  (%.1f%%)", stats.Wins, stats.Losses, winRate),
			"Current streak "+streak,
			fmt.Sprintf("Best streak    W%d", stats.LongestStreak),
			"",
			fmt.Sprintf("Collection     %d Monsters", stats.CollectionSize),
			fmt.Sprintf("Captures       %d", stats.Captures),
			fmt.Sprintf("Expeditions    %d", stats.Expeditions),
			fmt.Sprintf("Dojo clears    %d", stats.DojoClears),
			fmt.Sprintf("Mastery Marks  %d", stats.MasteryMarks),
			"",
			fmt.Sprintf("SSH sessions   %d", stats.Sessions),
			"Play time      "+compactDuration(stats.PlayTime),
		)
		if world := m.wb.worldStats; world != nil {
			lines = append(lines, "", titleStyle.Render("WORLD"),
				fmt.Sprintf("Registered Trainers  %d", world.RegisteredTrainers),
				fmt.Sprintf("Completed Battles    %d", world.CompletedBattles),
				fmt.Sprintf("Completed Activities %d", world.CompletedActivities),
			)
		}
	} else {
		lines = append(lines, dimStyle.Render("Loading authoritative statistics…"))
	}
	return strings.Join(append(lines, "", dimStyle.Render("S / esc  close")), "\n")
}

func (m Model) renderHelpModal() string {
	supportID := groupedSupportID(m.hash)
	return strings.Join([]string{
		titleStyle.Render("HELP & SUPPORT"),
		"",
		"Move around the Dojo with arrows, WASD, or HJKL.",
		"Press P for the Workbench and F to find a Battle.",
		"Stand beside a Trainer and press C to challenge them.",
		"Press S for lifetime Stats and ? for this Help screen.",
		"",
		titleStyle.Render("CONTACT SUPPORT"),
		"Email   " + supportEmail,
		"Issues  " + supportIssuesURL,
		"",
		"Include this information with your report:",
		"• Support ID: " + supportID,
		"• Any error reference shown by the game",
		"• The approximate UTC time",
		"• What you were doing when the problem occurred",
		"",
		dimStyle.Render("? / esc  close"),
	}, "\n")
}
