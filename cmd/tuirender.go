package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// How the tui draws itself, after k9s: header (info, hotkeys, logo), an
// optional prompt line, one bordered box holding the table, a text page,
// a dialog or the create form, and the crumbs line with the flash at its
// right.

// k9s is the colour scheme, named after what it imitates: orange logo
// and info keys, blue hotkeys and borders, aqua titles and cursor,
// black-on-aqua crumbs with the active one orange.
var k9s = struct {
	logo, infoKey, infoVal, hotKey, hotLabel, border, title, count,
	header, sortHeader, selected, crumb, crumbActive, flash, flashErr,
	dim, prompt, dialogBorder, dialogTitle, text lipgloss.Style
	state map[string]lipgloss.Style
}{
	logo:         lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	infoKey:      lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	infoVal:      lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
	hotKey:       lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true),
	hotLabel:     lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
	border:       lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
	title:        lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true),
	count:        lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	header:       lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true),
	sortHeader:   lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true),
	selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("51")),
	crumb:        lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("51")),
	crumbActive:  lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("214")),
	flash:        lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
	flashErr:     lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
	dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	prompt:       lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
	dialogBorder: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	dialogTitle:  lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
	text:         lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
	state: map[string]lipgloss.Style{
		"running":  lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
		"ACTIVE":   lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
		"imported": lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
		"paused":   lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		"stopped":  lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		"off":      lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		"BUILD":    lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
		"starting": lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
		"stopping": lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
		"dead":     lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		"failed":   lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		"private":  lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		"archived": lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		// :agents' CLAUDE column: green at work, yellow waiting for you.
		"working": lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
		"blocked": lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true),
		"done":    lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true),
	},
}

// stateStyle colours a STATE cell: the exact status, else by its rank
// (every provider's stopped/deallocated spelling is paused-yellow).
func stateStyle(status string) (lipgloss.Style, bool) {
	if st, ok := k9s.state[status]; ok {
		return st, true
	}
	switch vmStateRank(status) {
	case 1:
		return k9s.state["paused"], true
	case 2:
		return k9s.state["dead"], true
	}
	return lipgloss.Style{}, false
}

// tuiLogo is "onctl" in figlet's standard font.
var tuiLogo = []string{
	`                  _   _ `,
	`  ___  _ __   ___| |_| |`,
	` / _ \| '_ \ / __| __| |`,
	`| (_) | | | | (__| |_| |`,
	` \___/|_| |_|\___|\__|_|`,
}

// Layout: the header shows when there is room for it; the crumbs line
// is always the last.
const (
	tuiHeaderHeight = 5
	tuiMinForHeader = 18
)

// showHeader reports whether the header fits.
func (m tuiModel) showHeader() bool { return m.height >= tuiMinForHeader }

// promptHeight is the prompt line, when one is open.
func (m tuiModel) promptHeight() int {
	if m.filtering || m.commanding || m.tasking || m.filter.Value() != "" {
		return 1
	}
	return 0
}

// tableHeight is how many rows fit inside the box (its borders and the
// column header taken off).
func (m tuiModel) tableHeight() int {
	if m.height == 0 {
		return 20
	}
	h := m.height - 1 - m.promptHeight() // crumbs, prompt
	if m.showHeader() {
		h -= tuiHeaderHeight
	}
	return max(h-3, 1) // two borders and the header row
}

func (m tuiModel) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var b strings.Builder
	if m.showHeader() {
		b.WriteString(m.headerView() + "\n")
	}
	if m.promptHeight() > 0 {
		b.WriteString(m.promptView() + "\n")
	}
	b.WriteString(m.boxView())
	b.WriteString(m.crumbsView())
	return b.String()
}

// headerView is k9s's top block: info on the left, the view's hotkeys
// in the middle, the logo on the right.
func (m tuiModel) headerView() string {
	info := m.infoBlock()
	logo := make([]string, len(tuiLogo))
	for i, l := range tuiLogo {
		logo[i] = k9s.logo.Render(l)
	}
	infoW := 0
	for _, l := range info {
		infoW = max(infoW, lipgloss.Width(l))
	}
	logoW := lipgloss.Width(tuiLogo[0])
	keysW := m.width - infoW - logoW - 4
	var cols []string
	cols = append(cols, strings.Join(padLines(info, infoW, tuiHeaderHeight), "\n"))
	if keysW >= 16 {
		cols = append(cols, strings.Join(padLines(m.hotkeyBlock(keysW), keysW, tuiHeaderHeight), "\n"))
	}
	if m.width >= infoW+logoW+2 {
		cols = append(cols, strings.Join(padLines(logo, logoW, tuiHeaderHeight), "\n"))
	}
	rows := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, joinWithGap(cols, "  ")...), "\n")
	for i := range rows {
		rows[i] = fit(rows[i], m.width)
	}
	return strings.Join(rows, "\n")
}

// joinWithGap puts gap between blocks for JoinHorizontal.
func joinWithGap(blocks []string, gap string) []string {
	var out []string
	for i, b := range blocks {
		if i > 0 {
			out = append(out, strings.Repeat(gap+"\n", tuiHeaderHeight-1)+gap)
		}
		out = append(out, b)
	}
	return out
}

// padLines pads each line to w cells and the block to h lines.
func padLines(lines []string, w, h int) []string {
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = padRight(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// infoBlock is the header's left column: the provider, the VMs by
// state, what's in flight, when the list was last refreshed.
func (m tuiModel) infoBlock() []string {
	kv := func(k, v string) string { return k9s.infoKey.Render(padRight(k+":", 11)) + k9s.infoVal.Render(v) }
	live, paused, other := 0, 0, 0
	for _, vm := range m.vms {
		switch vmStateRank(vm.Status) {
		case 0:
			live++
		case 1:
			paused++
		default:
			other++
		}
	}
	vms := fmt.Sprintf("%d running, %d paused", live, paused)
	if other > 0 {
		vms += fmt.Sprintf(", %d other", other)
	}
	refreshed := "never"
	if !m.refreshedAt.IsZero() {
		refreshed = shortAgo(time.Since(m.refreshedAt))
	}
	var busy []string
	for name, verb := range m.pending {
		busy = append(busy, presentTense(verb)+" "+name)
	}
	sort.Strings(busy) // a map's order would shuffle them every spinner frame
	status := refreshed
	if len(busy) > 0 {
		status = m.spin.View() + " " + strings.Join(busy, ", ")
	} else if len(m.loading) > 0 {
		status = m.spin.View() + " " + refreshed
	}
	return []string{
		kv("Provider", orDash(cloudProvider)),
		kv("VMs", vms),
		kv("Showing", m.stateFilter.String()),
		kv("Refreshed", status),
		kv("Rev", Version),
	}
}

// hotkeyBlock is the header's middle column: the view's hotkeys then
// the common ones, in as many columns of tuiHeaderHeight as fit.
func (m tuiModel) hotkeyBlock(width int) []string {
	v := vmsView
	t := m.top()
	if t.kind == pageTable {
		v = t.view
	}
	keys := append([]hotkey{}, v.hotkeys...)
	if t.kind == pageText {
		keys = []hotkey{{"j/k", "scroll"}, {"g/G", "top/bottom"}, {"y", "json"}, {"s", "shell"}, {"a", "claude"}}
	}
	keys = append(keys, commonHotkeys...)
	// Column width from the widest entry.
	keyW, labelW := 0, 0
	for _, k := range keys {
		keyW = max(keyW, lipgloss.Width(k.key)+2)
		labelW = max(labelW, lipgloss.Width(k.label))
	}
	colW := keyW + 1 + labelW
	cols := max(1, (width+2)/(colW+2))
	lines := make([]string, tuiHeaderHeight)
	for i, k := range keys {
		col, row := i/tuiHeaderHeight, i%tuiHeaderHeight
		if col >= cols {
			break
		}
		cell := k9s.hotKey.Render(padRight("<"+k.key+">", keyW)) + " " + k9s.hotLabel.Render(padRight(k.label, labelW))
		if col > 0 {
			lines[row] += "  "
		}
		lines[row] += cell
	}
	return lines
}

// promptView is the line above the box while a prompt is open (or a
// filter is on): k9s's command/filter prompt.
func (m tuiModel) promptView() string {
	switch {
	case m.commanding:
		return fit(k9s.prompt.Render(":")+" "+m.command.View(), m.width)
	case m.filtering:
		return fit(k9s.prompt.Render("/")+" "+m.filter.View(), m.width)
	case m.tasking:
		return fit(k9s.prompt.Render(">")+" "+m.task.View(), m.width)
	}
	return fit(k9s.prompt.Render("/")+" "+m.filter.Value()+"  "+k9s.dim.Render("(esc clears)"), m.width)
}

// boxView is the bordered box: the table, a text page, the create form
// or a dialog inside it. Always m.height - header - prompt - crumbs
// lines high.
func (m tuiModel) boxView() string {
	inner := m.tableHeight() + 1 // plus the column header row
	w := max(m.width-2, 1)
	var title string
	var lines []string
	switch {
	case m.dialog != nil:
		title, lines = m.dialogTitleAndLines(w, inner)
	case m.create != nil:
		title = "Create a VM"
		lines = m.create.lines(w)
	case m.top().kind == pageText:
		title, lines = m.textPage(w, inner)
	default:
		title, lines = m.table(w, inner)
	}
	var b strings.Builder
	top := "─ " + title + " "
	b.WriteString(k9s.border.Render("┌") + fitPlain(top, w) + k9s.border.Render(strings.Repeat("─", max(w-lipgloss.Width(top), 0))+"┐") + "\n")
	for i := 0; i < inner; i++ {
		var l string
		if i < len(lines) {
			l = lines[i]
		}
		b.WriteString(k9s.border.Render("│") + padRight(fit(l, w), w) + k9s.border.Render("│") + "\n")
	}
	b.WriteString(k9s.border.Render("└"+strings.Repeat("─", w)+"┘") + "\n")
	return b.String()
}

// fitPlain cuts a styled title's plain border piece.
func fitPlain(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = fit(s, w)
	}
	// "─ " stays border-coloured; the title keeps its own colours.
	return k9s.border.Render("─ ") + strings.TrimPrefix(s, "─ ")
}

// table renders the top table view: its title with the count, the
// column header (sorted column marked) and the rows around the cursor.
func (m tuiModel) table(w, inner int) (string, []string) {
	t := m.top()
	rows := m.rows()
	title := k9s.title.Render(t.view.title)
	if t.view == vmsView {
		title += k9s.title.Render("(" + m.stateFilter.String() + ")")
	}
	if f := m.filter.Value(); f != "" {
		title += k9s.title.Render("</" + f + ">")
	}
	title += k9s.count.Render(fmt.Sprintf("[%d]", len(rows)))
	if len(rows) == 0 {
		msg := t.view.empty
		if err := m.listErr[t.view.resource]; err != nil {
			msg = fmt.Sprintf("Listing %s failed: %v. R retries.", t.view.resource, err)
		} else if !m.loaded[t.view.resource] {
			msg = "Loading…"
		} else if m.filter.Value() != "" {
			msg = "No rows match the filter."
		} else if t.view == vmsView && len(m.vms) > 0 {
			msg = "No " + m.stateFilter.String() + " VMs. Press 0 for all of them."
		}
		return title, []string{"", " " + k9s.dim.Render(msg)}
	}
	widths := make([]int, len(t.view.columns))
	for i, col := range t.view.columns {
		widths[i] = lipgloss.Width(col.title) + 1 // room for the sort arrow
	}
	for _, r := range rows {
		for i, c := range r.cells {
			if i < len(widths) {
				widths[i] = max(widths[i], lipgloss.Width(c))
			}
		}
	}
	var header []string
	for i, col := range t.view.columns {
		title := col.title
		style := k9s.header
		if t.sortCol == i {
			style = k9s.sortHeader
			if t.sortDesc {
				title += "↓"
			} else {
				title += "↑"
			}
		}
		header = append(header, style.Render(padRight(title, widths[i])))
	}
	lines := []string{" " + strings.Join(header, "  ")}

	// The window of rows that keeps the cursor on screen.
	top := 0
	if t.cursor >= inner-1 {
		top = t.cursor - (inner - 1) + 1
	}
	end := min(top+inner-1, len(rows))
	for r := top; r < end; r++ {
		row := rows[r]
		var cells []string
		for i := range t.view.columns {
			c := ""
			if i < len(row.cells) {
				c = row.cells[i]
			}
			cells = append(cells, padRight(c, widths[i]))
		}
		plain := " " + strings.Join(cells, "  ")
		if r == t.cursor {
			lines = append(lines, k9s.selected.Render(padRight(fit(plain, w), w)))
			continue
		}
		if i := t.view.stateCol; i >= 0 && i < len(cells) {
			if _, ok := m.pending[row.key]; ok {
				cells[i] = k9s.title.Render(cells[i])
			} else if st, ok := stateStyle(row.state); ok {
				cells[i] = st.Render(cells[i])
			}
		}
		lines = append(lines, " "+strings.Join(cells, "  "))
	}
	return title, lines
}

// textPage renders a describe/json/help page, scrolled.
func (m tuiModel) textPage(w, inner int) (string, []string) {
	t := m.top()
	title := k9s.title.Render(t.title)
	if len(t.lines) > inner {
		pct := 100
		if len(t.lines) > 0 {
			pct = min((t.scroll+inner)*100/len(t.lines), 100)
		}
		title += k9s.count.Render(fmt.Sprintf("[%d%%]", pct))
	}
	var lines []string
	for i := t.scroll; i < len(t.lines) && len(lines) < inner; i++ {
		lines = append(lines, " "+k9s.text.Render(t.lines[i]))
	}
	return title, lines
}

// dialogTitleAndLines draws the dialog centred in the box.
func (m tuiModel) dialogTitleAndLines(w, inner int) (string, []string) {
	d := m.dialog
	var body []string
	body = append(body, d.body...)
	body = append(body, "", dialogKeys(d))
	bw := lipgloss.Width(d.title)
	for _, l := range body {
		bw = max(bw, lipgloss.Width(l))
	}
	bw = min(bw+4, w-2)
	var box []string
	box = append(box, k9s.dialogBorder.Render("┌"+strings.Repeat("─", bw)+"┐"))
	box = append(box, k9s.dialogBorder.Render("│")+padRight("  "+k9s.dialogTitle.Render(d.title), bw)+k9s.dialogBorder.Render("│"))
	box = append(box, k9s.dialogBorder.Render("│")+strings.Repeat(" ", bw)+k9s.dialogBorder.Render("│"))
	for _, l := range body {
		box = append(box, k9s.dialogBorder.Render("│")+padRight("  "+fit(l, bw-2), bw)+k9s.dialogBorder.Render("│"))
	}
	box = append(box, k9s.dialogBorder.Render("└"+strings.Repeat("─", bw)+"┘"))
	pad := max((w-bw-2)/2, 0)
	topPad := max((inner-len(box))/2, 0)
	lines := make([]string, topPad)
	for _, l := range box {
		lines = append(lines, strings.Repeat(" ", pad)+l)
	}
	title := k9s.title.Render(m.pageTitle())
	return title, lines
}

// dialogKeys is the dialog's last line: its keys and what they do.
func dialogKeys(d *tuiDialog) string {
	if d.alert {
		return k9s.hotKey.Render("<enter>") + " OK"
	}
	if len(d.choices) == 0 {
		return k9s.hotKey.Render("<y>") + " yes   " + k9s.hotKey.Render("<n>") + " no"
	}
	var parts []string
	for _, c := range d.choices {
		parts = append(parts, k9s.hotKey.Render("<"+c.key+">")+" "+c.label)
	}
	parts = append(parts, k9s.hotKey.Render("<esc>")+" cancel")
	return strings.Join(parts, "   ")
}

// pageTitle is the top page's own title, for a dialog over it.
func (m tuiModel) pageTitle() string {
	t := m.top()
	if t.kind == pageText {
		return t.title
	}
	return t.view.title
}

// crumbsView is the bottom line: a chip per page, the current one
// orange, and the flash at the right.
func (m tuiModel) crumbsView() string {
	var chips []string
	for i, p := range m.pages {
		name := p.title
		if p.kind == pageTable {
			name = p.view.name
		}
		style := k9s.crumb
		if i == len(m.pages)-1 {
			style = k9s.crumbActive
		}
		chips = append(chips, style.Render(" <"+name+"> "))
	}
	if m.create != nil {
		chips = append(chips, k9s.crumbActive.Render(" <create> "))
	}
	left := strings.Join(chips, " ")
	// An error popup already says it; the flash below would repeat it.
	if m.flash == "" || (m.dialog != nil && m.dialog.alert) {
		return fit(left, m.width)
	}
	flash := k9s.flash.Render(m.flash)
	if m.flashErr {
		flash = k9s.flashErr.Render("✘ " + m.flash)
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(flash) - 1
	if gap < 1 {
		return fit(left+" "+flash, m.width)
	}
	return left + strings.Repeat(" ", gap) + flash
}

// shortAgo is "just now", "12s ago", "3m ago".
func shortAgo(d time.Duration) string {
	switch {
	case d < 2*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	default:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
}

// padRight pads s with spaces to w cells (no-op when it's wider).
func padRight(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// fit cuts s to w cells, ANSI styling kept.
func fit(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}
