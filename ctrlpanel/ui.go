package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// メッセージ型
type autoStopMsg struct{ id int }
type sensorTickMsg struct{}
type sensorResultMsg struct {
	data SensorData
	err  error
}
type soundDoneMsg struct {
	stats SongPlayStats
	err   error
}

// ── Styles ──

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	keyCapNormal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Foreground(lipgloss.Color("229")).
			Bold(true).
			Width(5).
			Align(lipgloss.Center)

	keyCapActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("229")).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("229")).
			Bold(true).
			Width(5).
			Align(lipgloss.Center)

	panelBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))

	logEntryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	txLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("80"))

	errLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	statusOn = lipgloss.NewStyle().
			Foreground(lipgloss.Color("78")).
			Bold(true)

	statusOff = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	sensorLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	sensorValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Bold(true)
)

// ── Screen: Device Select ──

type selectModel struct {
	ports     []string
	cursor    int
	soundFile string
}

func newSelectModel(ports []string, soundFile string) selectModel {
	return selectModel{
		ports:     ports,
		soundFile: soundFile,
	}
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.ports)-1 {
				m.cursor++
			}
		case "enter":
			port := m.ports[m.cursor]
			conn, err := openSerial(port)
			if err != nil {
				return m, tea.Quit
			}
			roomba := NewRoomba(conn)
			roomba.Start()
			roomba.Safe()
			return newPanelModel(port, roomba, m.soundFile), nil
		}
	}
	return m, nil
}

func (m selectModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Select Device "))
	b.WriteString("\n\n")

	for i, port := range m.ports {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "> "
			style = selectedStyle
		}
		b.WriteString(style.Render(cursor + port))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓: select  enter: connect  q: quit"))
	return b.String()
}

// ── Screen: Control Panel ──

const sensorPollInterval = 1 * time.Second

const (
	defaultTermWidth  = 100
	defaultTermHeight = 30

	minLeftPanelOuter  = 34
	minRightPanelOuter = 36
	minPanelOuterH     = 10
	panelGapWidth      = 1
	stackGapHeight     = 1
	minRenderableWidth = 56
	minRenderableH     = 18
)

type panelModel struct {
	port         string
	roomba       *Roomba
	logs         []string
	activeKeys   map[string]bool
	timerID      int
	stopped      bool
	mainBrush    bool
	vacuum       bool
	sideBrush    bool
	soundFile    string
	soundPlaying bool
	sensors      SensorData
	connected    bool
	pollErrors   int
	lastSensorAt time.Time
	width        int
	height       int
}

func newPanelModel(port string, roomba *Roomba, soundFile string) panelModel {
	logs := []string{logEntryStyle.Render("Connected to " + port)}
	if strings.TrimSpace(soundFile) != "" {
		logs = append(logs, logEntryStyle.Render("Sound file: "+soundFile))
	}

	return panelModel{
		port:         port,
		roomba:       roomba,
		logs:         logs,
		activeKeys:   map[string]bool{},
		stopped:      true,
		soundFile:    soundFile,
		connected:    true,
		lastSensorAt: time.Now(),
	}
}

func (m panelModel) Init() tea.Cmd {
	return tea.Batch(
		scheduleSensorPoll(),
		tea.WindowSize(),
	)
}

func scheduleSensorPoll() tea.Cmd {
	return tea.Tick(sensorPollInterval, func(time.Time) tea.Msg {
		return sensorTickMsg{}
	})
}

func (m panelModel) pollSensors() tea.Cmd {
	roomba := m.roomba
	return func() tea.Msg {
		data, err := roomba.QuerySensors()
		return sensorResultMsg{data: data, err: err}
	}
}

func (m panelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case sensorTickMsg:
		return m, m.pollSensors()

	case sensorResultMsg:
		if msg.err != nil {
			m.pollErrors++
			if time.Since(m.lastSensorAt) >= 5*sensorPollInterval {
				m.connected = false
			}
		} else {
			m.sensors = msg.data
			m.pollErrors = 0
			m.connected = true
			m.lastSensorAt = time.Now()
		}
		return m, scheduleSensorPoll()

	case soundDoneMsg:
		m.soundPlaying = false
		if msg.err != nil {
			m.appendLog(errLogStyle, "ERR: Sound: %v", msg.err)
			return m, nil
		}
		m.appendLog(
			txLogStyle,
			"TX: Sound done chunks=%d notes=%d elapsed=%s",
			msg.stats.Chunks,
			msg.stats.Notes,
			msg.stats.Elapsed.Round(time.Millisecond),
		)
		return m, nil

	case autoStopMsg:
		if msg.id == m.timerID {
			m.activeKeys = map[string]bool{}
			if !m.stopped {
				m.exec("Stop", m.roomba.DriveStop)
				m.stopped = true
			}
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "w", "W":
			m.activeKeys = map[string]bool{"W": true}
			m.exec("Forward", m.roomba.Forward)
			m.stopped = false
		case "ctrl+w":
			m.activeKeys = map[string]bool{"W": true, "Ctrl": true}
			m.exec("Forward+Left", m.roomba.ForwardLeft)
			m.stopped = false
		case "alt+w":
			m.activeKeys = map[string]bool{"W": true, "Alt": true}
			m.exec("Forward+Right", m.roomba.ForwardRight)
			m.stopped = false

		case "s", "S":
			m.activeKeys = map[string]bool{"S": true}
			m.exec("Backward", m.roomba.Backward)
			m.stopped = false
		case "ctrl+s":
			m.activeKeys = map[string]bool{"S": true, "Ctrl": true}
			m.exec("Backward+Left", m.roomba.BackwardLeft)
			m.stopped = false
		case "alt+s":
			m.activeKeys = map[string]bool{"S": true, "Alt": true}
			m.exec("Backward+Right", m.roomba.BackwardRight)
			m.stopped = false

		case "a", "A":
			m.activeKeys = map[string]bool{"Ctrl": true}
			m.exec("Turn Left", m.roomba.TurnLeft)
			m.stopped = false
		case "d", "D":
			m.activeKeys = map[string]bool{"Alt": true}
			m.exec("Turn Right", m.roomba.TurnRight)
			m.stopped = false

		case "0":
			m.exec("Start (Passive)", m.roomba.Start)
			return m, nil
		case "1":
			m.exec("Safe Mode", m.roomba.Safe)
			return m, nil
		case "2":
			m.exec("Full Mode", m.roomba.Full)
			return m, nil
		case "3":
			m.exec("Power Off", m.roomba.PowerOff)
			return m, nil
		case "m", "M":
			m.toggleMainBrush()
			return m, nil
		case "v", "V":
			m.toggleVacuum()
			return m, nil
		case "b", "B":
			m.toggleSideBrush()
			return m, nil
		case "x", "X":
			m.setMotors(false, false, false)
			return m, nil
		case "p", "P":
			if m.soundPlaying {
				m.appendLog(logEntryStyle, "Sound already playing")
				return m, nil
			}
			if strings.TrimSpace(m.soundFile) == "" {
				m.appendLog(errLogStyle, "ERR: sound file is not set (use --sound <file>)")
				return m, nil
			}
			m.soundPlaying = true
			m.appendLog(txLogStyle, "TX: Play sound file %s", m.soundFile)
			return m, m.playSoundCmd()

		default:
			return m, nil
		}

		m.timerID++
		id := m.timerID
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return autoStopMsg{id: id}
		})
	}
	return m, nil
}

func (m *panelModel) exec(name string, fn func() error) {
	if err := fn(); err != nil {
		m.appendLog(errLogStyle, "ERR: %s: %v", name, err)
		return
	}
	m.appendLog(txLogStyle, "TX: %s", name)
}

func (m *panelModel) appendLog(style lipgloss.Style, format string, args ...any) {
	m.logs = append(m.logs, style.Render(fmt.Sprintf(format, args...)))
	if len(m.logs) > 50 {
		m.logs = m.logs[len(m.logs)-50:]
	}
}

func (m *panelModel) toggleMainBrush() {
	m.setMotors(!m.mainBrush, m.vacuum, m.sideBrush)
}

func (m *panelModel) toggleVacuum() {
	m.setMotors(m.mainBrush, !m.vacuum, m.sideBrush)
}

func (m *panelModel) toggleSideBrush() {
	m.setMotors(m.mainBrush, m.vacuum, !m.sideBrush)
}

func (m *panelModel) setMotors(mainBrush, vacuum, sideBrush bool) {
	prevMain := m.mainBrush
	prevVacuum := m.vacuum
	prevSide := m.sideBrush

	m.mainBrush = mainBrush
	m.vacuum = vacuum
	m.sideBrush = sideBrush

	name := fmt.Sprintf("Motors main=%t vacuum=%t side=%t", mainBrush, vacuum, sideBrush)
	if err := m.roomba.SetMotors(mainBrush, vacuum, sideBrush); err != nil {
		m.mainBrush = prevMain
		m.vacuum = prevVacuum
		m.sideBrush = prevSide
		m.appendLog(errLogStyle, "ERR: %s: %v", name, err)
		return
	}

	m.appendLog(txLogStyle, "TX: %s", name)
}

func (m panelModel) playSoundCmd() tea.Cmd {
	roomba := m.roomba
	path := m.soundFile
	return func() tea.Msg {
		stats, err := roomba.PlaySongFile(path)
		return soundDoneMsg{
			stats: stats,
			err:   err,
		}
	}
}

func (m panelModel) renderKeys() string {
	cap := func(label, key string) string {
		if m.activeKeys[key] {
			return keyCapActive.Render(label)
		}
		return keyCapNormal.Render(label)
	}

	blank := lipgloss.NewStyle().Width(7).Height(3).Render("")
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, blank, cap("W", "W"))
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, cap("A", "Ctrl"), cap("S", "S"), cap("D", "Alt"))

	return lipgloss.JoinVertical(lipgloss.Left, row1, row2)
}

func (m panelModel) renderSensors() string {
	s := m.sensors
	line := func(label, value string) string {
		return sensorLabelStyle.Render(label) + sensorValueStyle.Render(value)
	}

	batPct := fmt.Sprintf("%d%%", s.BatteryPercent())
	voltage := fmt.Sprintf("%.1fV", float64(s.Voltage)/1000)
	temp := fmt.Sprintf("%d°C", s.Temperature)

	var b strings.Builder
	b.WriteString(line("Mode:  ", s.ModeName()) + "\n")
	b.WriteString(line("Batt:  ", fmt.Sprintf("%s  %s", batPct, voltage)) + "\n")
	b.WriteString(line("Temp:  ", temp) + "\n")
	b.WriteString(line("Charge:", fmt.Sprintf(" %s", s.ChargingStateName())))

	return b.String()
}

func (m panelModel) renderMotors() string {
	onOff := func(v bool) string {
		if v {
			return statusOn.Render("ON")
		}
		return statusOff.Render("OFF")
	}

	var b strings.Builder
	b.WriteString(sensorLabelStyle.Render("Main: ") + onOff(m.mainBrush) + "\n")
	b.WriteString(sensorLabelStyle.Render("Vac:  ") + onOff(m.vacuum) + "\n")
	b.WriteString(sensorLabelStyle.Render("Side: ") + onOff(m.sideBrush))
	return b.String()
}

func (m panelModel) renderSound() string {
	state := statusOff.Render("Idle")
	if m.soundPlaying {
		state = statusOn.Render("Playing")
	}
	path := m.soundFile
	if strings.TrimSpace(path) == "" {
		path = "(none)"
	}

	var b strings.Builder
	b.WriteString(sensorLabelStyle.Render("State: ") + state + "\n")
	b.WriteString(dimStyle.Render(path))
	return b.String()
}

func (m panelModel) terminalSize() (int, int) {
	w := m.width
	h := m.height
	if w <= 0 {
		w = defaultTermWidth
	}
	if h <= 0 {
		h = defaultTermHeight
	}
	return w, h
}

func panelInnerSize(style lipgloss.Style, outerWidth, outerHeight int) (int, int) {
	if outerWidth < 1 {
		outerWidth = 1
	}
	if outerHeight < 1 {
		outerHeight = 1
	}

	// lipgloss の Width/Height は padding を含むため、外形指定時は border 分だけ差し引く。
	innerW := outerWidth - style.GetHorizontalBorderSize()
	innerH := outerHeight - style.GetVerticalBorderSize()
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	return innerW, innerH
}

func renderPanel(style lipgloss.Style, outerWidth, outerHeight int, content string) string {
	innerW, innerH := panelInnerSize(style, outerWidth, outerHeight)
	contentH := innerH - style.GetPaddingTop() - style.GetPaddingBottom()
	if contentH < 1 {
		contentH = 1
	}
	lines := strings.Split(content, "\n")
	if len(lines) > contentH {
		lines = lines[:contentH]
	}
	clipped := strings.Join(lines, "\n")
	return style.
		Copy().
		Width(innerW).
		Height(innerH).
		Render(clipped)
}

func tailStrings(items []string, n int) []string {
	if n <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	return string(rs[:maxLen-1]) + "…"
}

func (m panelModel) renderStatusBar(termWidth int) string {
	main := "● " + m.port
	mainStyle := statusOn
	if !m.connected {
		main = "○ " + m.port + " (disconnected)"
		mainStyle = statusOff
	}

	const hint = "Esc: quit"
	if termWidth > 0 {
		// " " + main + "  " + hint が幅を超えないよう主メッセージだけ詰める。
		maxMain := termWidth - 1 - 2 - len([]rune(hint))
		if maxMain < 4 {
			maxMain = 4
		}
		main = truncateRunes(main, maxMain)
	}
	return " " + mainStyle.Render(main) + "  " + dimStyle.Render(hint)
}

func (m panelModel) renderResizeHint(termW, termH int) string {
	msg := fmt.Sprintf(
		"Terminal too small: %dx%d\nPlease resize to at least %dx%d",
		termW,
		termH,
		minRenderableWidth,
		minRenderableH,
	)

	boxStyle := panelBorder.Copy().Padding(1, 2)
	innerW, innerH := panelInnerSize(boxStyle, termW, termH-1)
	return boxStyle.Copy().Width(innerW).Height(innerH).Render(msg) + "\n" + dimStyle.Render(" Esc: quit")
}

func (m panelModel) View() string {
	termW, termH := m.terminalSize()
	if termW < minRenderableWidth || termH < minRenderableH {
		return m.renderResizeHint(termW, termH)
	}

	layoutW := termW
	if layoutW > 2 {
		// 端末最終桁の自動折り返しを避けるため1桁余白を残す。
		layoutW--
	}
	bodyH := termH - 1 // status bar 用に1行確保
	if bodyH < minPanelOuterH {
		bodyH = minPanelOuterH
	}

	// ── Left: Controls + Sensors ──
	keys := m.renderKeys()
	driveHints := hintStyle.Render(
		"  W/S        = Fwd/Back\n" +
			"  Ctrl+W/S   = +Left\n" +
			"  Alt+W/S    = +Right\n" +
			"  A/D        = Turn L/R")
	modeHints := hintStyle.Render(
		"  0 Passive  1 Safe\n" +
			"  2 Full     3 Power Off")
	motorHints := hintStyle.Render(
		"  M Main  V Vacuum\n" +
			"  B Side  X Motor Off")
	soundHints := hintStyle.Render("  P Play Sound")

	leftContent := lipgloss.JoinVertical(
		lipgloss.Left,
		keys,
		"",
		driveHints,
		"",
		modeHints,
		"",
		motorHints,
		"",
		m.renderMotors(),
		"",
		soundHints,
		"",
		m.renderSound(),
		"",
		m.renderSensors(),
	)

	leftStyle := panelBorder.Copy().Padding(1, 2)
	rightStyle := panelBorder.Copy().Padding(0, 1)

	var body string
	minTwoColWidth := minLeftPanelOuter + panelGapWidth + minRightPanelOuter
	if layoutW >= minTwoColWidth {
		available := layoutW - panelGapWidth
		leftOuter := int(float64(available) * 0.42)
		if leftOuter < minLeftPanelOuter {
			leftOuter = minLeftPanelOuter
		}
		if leftOuter > available-minRightPanelOuter {
			leftOuter = available - minRightPanelOuter
		}
		rightOuter := available - leftOuter

		_, rightInnerH := panelInnerSize(rightStyle, rightOuter, bodyH)
		logContent := strings.Join(tailStrings(m.logs, rightInnerH), "\n")

		leftPanel := renderPanel(leftStyle, leftOuter, bodyH, leftContent)
		rightPanel := renderPanel(rightStyle, rightOuter, bodyH, logContent)
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, strings.Repeat(" ", panelGapWidth), rightPanel)
	} else {
		stackBodyH := bodyH
		minStackH := (minPanelOuterH * 2) + stackGapHeight
		if stackBodyH < minStackH {
			stackBodyH = minStackH
		}

		topH := (stackBodyH - stackGapHeight) / 2
		bottomH := stackBodyH - stackGapHeight - topH
		if topH < minPanelOuterH {
			topH = minPanelOuterH
		}
		if bottomH < minPanelOuterH {
			bottomH = minPanelOuterH
		}

		_, rightInnerH := panelInnerSize(rightStyle, layoutW, bottomH)
		logContent := strings.Join(tailStrings(m.logs, rightInnerH), "\n")

		leftPanel := renderPanel(leftStyle, layoutW, topH, leftContent)
		rightPanel := renderPanel(rightStyle, layoutW, bottomH, logContent)
		body = lipgloss.JoinVertical(lipgloss.Left, leftPanel, "", rightPanel)
	}

	return body + "\n" + m.renderStatusBar(layoutW)
}
