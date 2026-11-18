package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jasonpiedrasantasdk/netportid/tracker"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time
type tickResetMsg time.Time

type errMsg struct{ error }

type searchState int

const (
	searchIdle searchState = iota
	searchDialog
	searchResult
)

type Model struct {
	store   *tracker.PortStore
	procMap *tracker.ProcMapper

	portList  []*tracker.PortEntry
	selected  int
	width     int
	height    int
	ready     bool

	ifaces       []string
	currentIface string
	paused       bool

	showAll bool

	searchState searchState
	searchQuery string
	searchPort  uint16
	searchFound *tracker.PortEntry

	err error
}

func New(store *tracker.PortStore, procMap *tracker.ProcMapper, ifaces []string) Model {
	return Model{
		store:        store,
		procMap:      procMap,
		ifaces:       ifaces,
		currentIface: "all",
		selected:     0,
		showAll:      false,
		searchState:  searchIdle,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.tick(),
	)
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) refreshPorts() {
	conns := m.store.GetPorts()
	m.portList = conns
}

func (m *Model) updateProcesses() {
	if m.procMap == nil {
		return
	}
	for _, port := range m.portList {
		if port.PID == 0 {
			entry, found := m.procMap.Lookup(port.LocalPort, port.Protocol)
			if found {
				m.store.SetProcess(port.Key, entry.PID, entry.Name, entry.CmdLine)
				port.PID = entry.PID
				port.ProcessName = entry.Name
				port.ProcessCmd = entry.CmdLine
			}
		}
	}
}

func (m *Model) filterPorts() []*tracker.PortEntry {
	if m.portList == nil {
		return nil
	}

	if m.searchState == searchResult && m.searchFound != nil {
		return []*tracker.PortEntry{m.searchFound}
	}

	if m.showAll {
		pm := m.procMap.GetAll()
		m.store.SeedFromProcMap(pm)
		return m.store.GetPorts()
	}

	return m.store.GetActivePorts()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case tea.KeyMsg:
		if m.searchState == searchDialog {
			switch msg.String() {
			case "esc":
				m.searchState = searchIdle
				m.searchQuery = ""
				m.searchPort = 0
				m.searchFound = nil
			case "enter":
				if m.searchQuery != "" {
					p, err := strconv.ParseUint(m.searchQuery, 10, 16)
					if err == nil {
						m.searchPort = uint16(p)
						pm := m.procMap.GetAll()
						found := m.store.SearchPort(m.searchPort, pm)
						if found != nil {
							m.searchFound = found
						} else {
							m.searchFound = &tracker.PortEntry{
								LocalPort: m.searchPort,
								Protocol:  6,
							}
						}
						m.searchState = searchResult
					}
				}
				m.searchQuery = ""
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				}
			default:
				for _, r := range msg.Runes {
					if r >= '0' && r <= '9' {
						m.searchQuery += string(r)
					}
				}
			}
			break
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "p":
			m.paused = !m.paused

		case "a":
			m.showAll = !m.showAll
			if m.showAll {
				pm := m.procMap.GetAll()
				m.store.SeedFromProcMap(pm)
			}
			m.selected = 0

		case "up", "k":
			m.selected--
			if m.selected < 0 {
				m.selected = 0
			}

		case "down", "j":
			conns := m.filterPorts()
			m.selected++
			if m.selected >= len(conns) {
				m.selected = len(conns) - 1
				if m.selected < 0 {
					m.selected = 0
				}
			}

		case "g":
			m.selected = 0

		case "b":
			conns := m.filterPorts()
			m.selected = len(conns) - 1
			if m.selected < 0 {
				m.selected = 0
			}

		case "s":
			if m.searchState == searchIdle {
				m.searchState = searchDialog
				m.searchQuery = ""
				m.searchPort = 0
				m.searchFound = nil
			}

		case "r":
			if m.procMap != nil {
				m.procMap.Update()
				m.updateProcesses()
			}

		case "?":
			return m, nil
		}

	case tickMsg:
		if !m.paused {
			m.refreshPorts()
			if m.procMap != nil {
				m.procMap.Update()
			}
			m.updateProcesses()
		}
		m.store.ResetTickDeltas()
		cmds = append(cmds, m.tick())

	case errMsg:
		m.err = msg.error

	case tea.QuitMsg:
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "Starting netPortID..."
	}

	if m.searchState == searchDialog {
		return m.searchOverlay()
	}

	var b strings.Builder

	b.WriteString(m.headerView())
	b.WriteString("\n")

	ports := m.filterPorts()

	colHeaders := m.columnHeaderView()
	if colHeaders != "" {
		b.WriteString(colHeaders)
		b.WriteString("\n")
	}

	listHeight := m.height - 4
	if listHeight < 3 {
		listHeight = 3
	}

	tableContent := m.portTableView(ports, listHeight)
	table := connListStyle.
		Width(m.width - 4).
		Height(listHeight).
		Render(tableContent)
	b.WriteString(table)
	b.WriteString("\n")

	b.WriteString(m.statusView())

	return b.String()
}

func (m Model) headerView() string {
	left := " netPortID"
	left += "  [" + m.currentIface + "]"

	if m.showAll {
		left += "  all:✓"
	} else {
		left += "  all:✕"
	}

	count := m.store.Count()
	filtered := len(m.filterPorts())

	right := fmt.Sprintf("%d", count)
	if filtered < count {
		right = fmt.Sprintf("%d/%d", filtered, count)
	}
	right += " ports"

	if m.paused {
		right += "  PAUSED"
	}

	return headerBarStyle.
		Width(m.width - 4).
		Render(left + "  │  " + right)
}

func (m Model) columnHeaderView() string {
	availWidth := m.width - 8
	if availWidth < 50 {
		return ""
	}

	colPort := 6
	colProto := 6
	colDir := 3
	colTX := 8
	colRX := 8
	colProc := 22
	colSep := 3
	fixedWidth := colPort + colProto + colDir + colTX + colRX + colProc + colSep*5
	remaining := availWidth - fixedWidth
	if remaining < 0 {
		remaining = 0
	}

	headerSep := mutedStyle.Render(strings.Repeat("─", availWidth))

	headPort := lipgloss.NewStyle().Foreground(colHeader).Bold(true).Width(colPort).Render("Port")
	headProto := lipgloss.NewStyle().Foreground(colHeader).Bold(true).Width(colProto).Render("Proto")
	headDir := lipgloss.NewStyle().Foreground(colHeader).Bold(true).Width(colDir).Render("Dir")
	headTX := lipgloss.NewStyle().Foreground(colHeader).Bold(true).Width(colTX).Align(lipgloss.Right).Render("TX")
	headRX := lipgloss.NewStyle().Foreground(colHeader).Bold(true).Width(colRX).Align(lipgloss.Right).Render("RX")
	headProc := lipgloss.NewStyle().Foreground(colHeader).Bold(true).Width(colProc + remaining).Render("Process")
	sep := mutedStyle.Render("│")

	return lipgloss.JoinHorizontal(lipgloss.Left, "  ",
		headPort, sep,
		headProto, sep,
		headDir, sep,
		headTX, sep,
		headRX, sep,
		headProc,
	) + "\n" + headerSep
}

func (m Model) portTableView(ports []*tracker.PortEntry, height int) string {
	if ports == nil || len(ports) == 0 {
		return mutedStyle.Render(" No ports yet. Waiting for traffic...")
	}

	var b strings.Builder

	availWidth := m.width - 8
	if availWidth < 50 {
		return " Terminal too narrow"
	}

	colPort := 6
	colProto := 6
	colDir := 3
	colTX := 8
	colRX := 8
	colProc := 22
	colSep := 3
	fixedWidth := colPort + colProto + colDir + colTX + colRX + colProc + colSep*5
	remaining := availWidth - fixedWidth
	if remaining < 0 {
		remaining = 0
	}
	colProc += remaining

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].LastSeen.After(ports[j].LastSeen)
	})

	start, end := 0, len(ports)
	if end > height {
		end = height
	}
	scrollOffset := 0
	if m.selected >= end && len(ports) > 0 {
		scrollOffset = m.selected - height + 1
	}
	start += scrollOffset
	end += scrollOffset
	if end > len(ports) {
		end = len(ports)
	}
	if start < 0 {
		start = 0
	}

	for i := start; i < end; i++ {
		p := ports[i]
		col := m.portCols(p, colPort, colProto, colDir, colTX, colRX, colProc)
		row := lipgloss.JoinHorizontal(lipgloss.Left, col...)
		if i == m.selected {
			b.WriteString("▸ ")
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString("  ")
			b.WriteString(normalStyle.Render(row))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) portCols(p *tracker.PortEntry, wPort, wProto, wDir, wTX, wRX, wProc int) []string {
	portStr := fmt.Sprintf("%d", p.LocalPort)

	proto := ProtoName(p.Protocol, p.ProtoInfo.Protocol)
	protoStr := truncate(proto, wProto-1)

	var dirSymbol, dirColor string
	dirStyle := activityStyle(p.TickBytesSent, p.TickBytesRecv)
	if p.TickBytesSent > 0 && p.TickBytesRecv > 0 {
		dirSymbol = "⇅"
	} else if p.TickBytesSent > 0 {
		dirSymbol = "↑"
	} else if p.TickBytesRecv > 0 {
		dirSymbol = "↓"
	} else if p.IsListening {
		dirSymbol = "●"
	} else {
		dirSymbol = "·"
	}
	_ = dirColor

	tx := formatBytes(p.BytesSent)
	rx := formatBytes(p.BytesRecv)
	if p.BytesSent == 0 && p.BytesRecv == 0 && p.IsListening {
		tx = "—"
		rx = "—"
	}

	proc := "?"
	if p.ProcessName != "" {
		proc = truncate(p.ProcessName, wProc-1)
	}

	sep := mutedStyle.Render("│")

	col1 := normalStyle.Width(wPort).Render(portStr)
	col2 := protoStyle(proto).Width(wProto).Render(protoStr)
	col3 := dirStyle.Width(wDir).Render(dirSymbol)
	col4 := normalStyle.Width(wTX).Align(lipgloss.Right).Render(tx)
	col5 := normalStyle.Width(wRX).Align(lipgloss.Right).Render(rx)
	col6 := normalStyle.Width(wProc).Render(proc)

	return []string{col1, sep, col2, sep, col3, sep, col4, sep, col5, sep, col6}
}

func ProtoName(protocol uint8, detected string) string {
	if detected != "" && detected != "?" && detected != "TCP" && detected != "UDP" {
		return detected
	}
	switch protocol {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	case 58:
		return "ICMPv6"
	default:
		return fmt.Sprintf("%d", protocol)
	}
}

func (m Model) searchOverlay() string {
	var b strings.Builder

	prompt := fmt.Sprintf("Enter port number:\n\n  %s█", m.searchQuery)

	s := dialogStyle.Render(
		dialogTitle.Render("Search Port") + "\n\n" +
			prompt + "\n\n" +
			hotkeyLabel("enter", "search") + "  " +
			hotkeyLabel("esc", "cancel"),
	)
	b.WriteString(m.centerOverlay(s))

	return b.String()
}

func hotkeyLabel(key, label string) string {
	k := lipgloss.NewStyle().Foreground(colYellow).Bold(true).Render(key)
	return fmt.Sprintf("(%s)%s", k, label)
}

func (m Model) centerOverlay(content string) string {
	lines := strings.Split(content, "\n")
	maxW := 0
	for _, l := range lines {
		clean := lipgloss.Width(l)
		if clean > maxW {
			maxW = clean
		}
	}

	vPad := (m.height - len(lines)) / 2
	if vPad < 0 {
		vPad = 0
	}

	out := strings.Repeat("\n", vPad)
	for _, l := range lines {
		pad := (m.width - lipgloss.Width(l)) / 2
		if pad < 0 {
			pad = 0
		}
		out += strings.Repeat(" ", pad) + l + "\n"
	}
	return out
}

func (m Model) statusView() string {
	ports := m.filterPorts()

	var left, right string

	count := m.store.Count()
	filtered := len(ports)
	if filtered < count {
		left = fmt.Sprintf("%d/%d ports", filtered, count)
	} else {
		left = fmt.Sprintf("%d ports", count)
	}

	if m.searchState == searchResult && m.searchFound != nil {
		found := m.searchFound
		if found.ProcessName != "" || found.PID > 0 {
			left = fmt.Sprintf("Port %d (%s) — %s (PID %d)", found.LocalPort, ProtoName(found.Protocol, found.ProtoInfo.Protocol), found.ProcessName, found.PID)
		} else if found.BytesSent > 0 || found.BytesRecv > 0 {
			left = fmt.Sprintf("Port %d — active", found.LocalPort)
		} else {
			left = fmt.Sprintf("Port %d — not found", found.LocalPort)
		}
	}

	paused := ""
	if m.paused {
		paused = pausedStyle.Render(" PAUSED ") + " "
	}

	right = hotkeyLabel("q", "uit") + " "
	right += hotkeyLabel("a", "ll") + " "
	right += hotkeyLabel("p", "ause") + " "
	right += hotkeyLabel("s", "earch") + " "
	right += hotkeyLabel("g", "top") + " "
	right += hotkeyLabel("b", "ottom") + " "
	right += hotkeyLabel("r", "efresh") + " "
	right += hotkeyLabel("?", "help")

	return statusBarStyle.
		Width(m.width - 4).
		Render(paused + left + "  │  " + right)
}
