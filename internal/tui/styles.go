package tui

import "charm.land/lipgloss/v2"

// Semantic styles for the shared chrome. Colors reinforce meaning that the
// text already carries, so a monochrome terminal loses no information.
var (
	titleStyle     = lipgloss.NewStyle().Bold(true)
	viewStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	transportStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	contextStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	causeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bodyStyle      = lipgloss.NewStyle()
	loadingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errorStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	staleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
)

// markerStyle returns the style of the freshness marker.
func markerStyle(freshness Freshness) lipgloss.Style {
	switch freshness {
	case FreshnessLoading:
		return loadingStyle
	case FreshnessError:
		return errorStyle
	case FreshnessStale:
		return staleStyle
	default:
		return contextStyle
	}
}
