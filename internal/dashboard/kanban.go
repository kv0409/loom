package dashboard

func (m Model) renderKanban() string {
	return panel("KANBAN", renderEmpty("Kanban board coming soon", availableWidth(m.width)), panelWidth(m.width))
}
