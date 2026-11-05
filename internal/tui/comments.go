package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"cairn/internal/i18n"
	"cairn/internal/images"
	"cairn/internal/progress"
	"cairn/internal/prthread"
)

// The comments view: a PR's whole conversation, full screen, with images one
// click away.

type threadMsg struct {
	key    string
	thread *prthread.Thread
	err    error
}

type imagesMsg struct {
	files, captions []string
	start           int
	err             error
}

var sImg = lipgloss.NewStyle().Foreground(accent).Underline(true)

// prURLOf returns the open PR of the selected chantier, else its latest one.
func (m Model) prURLOf() (string, int) {
	c := m.selected()
	if c == nil || c.Issue == nil || len(c.Issue.PullRequests) == 0 {
		return "", 0
	}
	pr := c.Issue.PullRequests[len(c.Issue.PullRequests)-1]
	if o := c.OpenPR(); o != nil {
		pr = *o
	}
	return pr.URL, pr.Number
}

func (m Model) openComments() (tea.Model, tea.Cmd) {
	url, _ := m.prURLOf()
	if url == "" {
		m.say(i18n.T("Pas de PR pour ce chantier.", "No PR for this ticket."), nil)
		return m, nil
	}
	key := m.selected().Key
	m.view, m.cscroll, m.showBots = "comments", 0, false
	if m.thread != nil && m.threadKey == key {
		m.scrollComments(m.firstNew())
		return m, nil
	}
	m.thread, m.threadKey, m.threadErr = nil, key, nil
	return m, loadThread(key, url)
}

func loadThread(key, url string) tea.Cmd {
	return func() tea.Msg {
		t, err := prthread.Fetch(url)
		if err == nil {
			markBroken(t)
		}
		return threadMsg{key: key, thread: t, err: err}
	}
}

// markBroken tries the repo-relative images (usually "./shot.png" written by
// an agent): those GitHub cannot serve are flagged instead of shown as images.
func markBroken(t *prthread.Thread) {
	t.Broken = map[int]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, src := range t.Images {
		if !prthread.Relative(src) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := images.Fetch(src, images.Source{Owner: t.Owner, Repo: t.Repo, Ref: t.HeadRef}); err != nil {
				mu.Lock()
				t.Broken[i+1] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

// openImages downloads srcs (in parallel) and hands them to the viewer.
func openImages(srcs, captions []string, from images.Source, start int) tea.Cmd {
	return func() tea.Msg {
		files := make([]string, len(srcs))
		errs := make([]error, len(srcs))
		var wg sync.WaitGroup
		for i, s := range srcs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				files[i], errs[i] = images.Fetch(s, from)
			}()
		}
		wg.Wait()
		var okFiles, okCaps []string
		newStart, failed := 0, 0
		for i := range srcs {
			if errs[i] != nil {
				failed++
				continue
			}
			if i == start {
				newStart = len(okFiles)
			}
			okFiles = append(okFiles, files[i])
			okCaps = append(okCaps, captions[i])
		}
		if len(okFiles) == 0 {
			return imagesMsg{err: fmt.Errorf(i18n.T("aucune image téléchargée (%v)", "no image downloaded (%v)"), errs[0])}
		}
		msg := imagesMsg{files: okFiles, captions: okCaps, start: newStart}
		if failed > 0 {
			msg.err = fmt.Errorf(i18n.T("%d image(s) inaccessibles", "%d image(s) unreachable"), failed)
		}
		return msg
	}
}

func (m Model) showImages(msg imagesMsg) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		self = "cairn"
	}
	args := append([]string{"img", "-start", strconv.Itoa(msg.start), "--"}, msg.files...)
	cmd := exec.Command(self, args...)
	cmd.Env = append(os.Environ(), "CAIRN_CAPTIONS="+strings.Join(msg.captions, "\n"))
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return doneMsg{err: err} })
}

func (m Model) threadImages(start int) (tea.Model, tea.Cmd) {
	t := m.thread
	if t == nil || len(t.Images) == 0 {
		m.say(i18n.T("Pas d'image dans cette PR.", "No image in this PR."), nil)
		return m, nil
	}
	var srcs, caps []string
	newStart := 0
	for i := range t.Images {
		if t.Broken[i+1] {
			continue
		}
		if i == start {
			newStart = len(srcs)
		}
		srcs = append(srcs, t.Images[i])
		caps = append(caps, fmt.Sprintf("image %d · PR #%d", i+1, t.Number))
		for _, e := range t.Entries {
			if strings.Contains(e.Body, fmt.Sprintf("[image %d]", i+1)) {
				caps[len(caps)-1] += " · " + e.Author + " (" + e.Kind + ")"
				break
			}
		}
	}
	if len(srcs) == 0 {
		m.say(i18n.T("Toutes les images de cette PR sont des liens cassés.", "Every image of this PR is a broken link."), nil)
		return m, nil
	}
	m.busy = fmt.Sprintf(i18n.T("Téléchargement de %d image(s)…", "Downloading %d image(s)…"), len(srcs))
	return m, openImages(srcs, caps, images.Source{Owner: t.Owner, Repo: t.Repo, Ref: t.HeadRef}, newStart)
}

// sendLabel ends each comment's header: a click forwards it to the agent.
// It reads the same in both languages, so the hit test can rely on it.
const sendLabel = "[→ agent]"

// backLabel is the comments view's top-left link back to the main screen.
func backLabel() string { return i18n.T("← Retour", "← Back") }

// commentsLayout renders the conversation; targets give the image (1-based)
// a line opens, 0 for none, and -(i+1) for the header of entry i.
func (m Model) commentsLayout(w int) ([]string, []int) {
	var lines []string
	var targets []int
	add := func(img int, s ...string) {
		for _, l := range s {
			lines = append(lines, l)
			targets = append(targets, img)
		}
	}
	if m.threadErr != nil {
		add(0, sErr.Render(m.threadErr.Error()))
		return lines, targets
	}
	t := m.thread
	if t == nil {
		add(0, sMuted.Render(i18n.T("Chargement de la conversation…", "Loading the conversation…")))
		return lines, targets
	}
	since, tracked := time.Time{}, false
	if m.seen != nil {
		since, tracked = m.seen.get(m.threadKey)
	}
	for i, e := range t.Entries {
		head := sBold.Render(e.Author)
		if e.Bot {
			head = sMuted.Render(e.Author + " (bot)")
		}
		kind := e.Kind
		switch e.State {
		case "APPROVED":
			kind = sGood.Render(i18n.T("review approuvée", "review approved"))
		case "CHANGES_REQUESTED":
			kind = sErr.Render(i18n.T("changements demandés", "changes requested"))
		}
		if e.Kind == "inline" {
			kind = sMuted.Render("inline ") + lipgloss.NewStyle().Foreground(working).Render(fmt.Sprintf("%s:%d", e.Path, e.Line))
		}
		header := head + sMuted.Render(" · ") + kind + sMuted.Render(" · "+progress.Ago(e.At))
		if tracked && e.Kind != "description" && e.At.After(since) {
			header = sFresh.Render("● ") + header + sFresh.Render(i18n.T("  nouveau", "  new"))
		}
		if e.Kind == "description" {
			add(0, header)
		} else {
			add(-(i + 1), header+"  "+sKey.Render(sendLabel))
		}
		body := renderMarkdown(e.Body, w)
		if e.Bot && !m.showBots && len(body) > 8 {
			hidden := len(body) - 8
			body = append(body[:8], sMuted.Render(fmt.Sprintf(i18n.T("  … %d lignes de bot masquées (bouton Bots)", "  … %d bot lines hidden (Bots button)"), hidden)))
		}
		for _, l := range body {
			img := 0
			if mm := prthread.ImageRe.FindStringSubmatch(xansi.Strip(l)); mm != nil {
				img, _ = strconv.Atoi(mm[1])
				// glamour splits "[image N]" into several styled runs: restyle
				// the line from plain text so the marker reads as one link.
				l = prthread.ImageRe.ReplaceAllStringFunc(strings.TrimRight(xansi.Strip(l), " "), func(s string) string {
					n, _ := strconv.Atoi(prthread.ImageRe.FindStringSubmatch(s)[1])
					if t.Broken[n] {
						return sErr.Render(fmt.Sprintf(i18n.T("[img %d : lien cassé %s]", "[img %d: broken link %s]"), n, t.Images[n-1]))
					}
					return sImg.Render(fmt.Sprintf("[img %d]", n))
				})
				if t.Broken[img] {
					img = 0
				}
			} else if e.Bot {
				l = sMuted.Render(xansi.Strip(l))
			}
			add(img, l)
		}
		add(0, "")
	}
	return lines, targets
}

// firstNew is the line of the first entry newer than your last visit, so
// the conversation opens where you left it; 0 when nothing is new.
func (m Model) firstNew() int {
	if m.thread == nil || m.seen == nil {
		return 0
	}
	since, ok := m.seen.get(m.threadKey)
	if !ok {
		return 0
	}
	_, targets := m.commentsLayout(m.width)
	for i, e := range m.thread.Entries {
		if e.Kind == "description" || !e.At.After(since) || (e.Bot && !m.showBots) {
			continue
		}
		for l, t := range targets {
			if t == -(i + 1) {
				return l
			}
		}
	}
	return 0
}

func (m Model) commentsView() string {
	_, num := m.prURLOf()
	head := sKey.Render(backLabel()) + "   " + sTitle.Render(fmt.Sprintf("PR #%d", num))
	if c := m.selected(); c != nil {
		head += sMuted.Render(" · " + c.Key + " · " + trunc(c.Title(), max(10, m.width/2)))
	}
	if t := m.thread; t != nil {
		n := 0
		for _, e := range t.Entries {
			if e.Kind != "description" && !e.Bot {
				n++
			}
		}
		head += sMuted.Render(fmt.Sprintf(i18n.T(" · %d commentaires · %d images", " · %d comments · %d images"), n, len(t.Images)-len(t.Broken)))
		if len(t.Broken) > 0 {
			head += sErr.Render(fmt.Sprintf(i18n.T(" · %d liens d'image cassés", " · %d broken image links"), len(t.Broken)))
		}
	}
	bodyH := m.bodyHeight()
	lines, _ := m.commentsLayout(m.width)
	start := min(m.cscroll, max(0, len(lines)-bodyH))
	lines = lines[start:]
	if len(lines) > bodyH {
		lines = lines[:bodyH]
	}
	for i, l := range lines {
		lines[i] = xansi.Truncate(l, m.width, "…")
	}
	body := lipgloss.NewStyle().Width(m.width).Height(bodyH).MaxHeight(bodyH).Render(strings.Join(lines, "\n"))
	return xansi.Truncate(head, m.width, "…") + "\n" + m.viewFlash() + "\n" + body + "\n" + m.viewFooter()
}

func (m Model) commentsButtons() []button {
	n := 0
	if m.thread != nil {
		n = len(m.thread.Images) - len(m.thread.Broken)
	}
	bots := "Bots"
	if m.showBots {
		bots = i18n.T("Masquer bots", "Hide bots")
	}
	return []button{
		{"esc", i18n.T("Retour", "Back"), true},
		{"i", fmt.Sprintf("Images (%d)", n), n > 0},
		{"b", bots, m.thread != nil},
		{"o", i18n.T("Ouvrir sur GitHub", "Open on GitHub"), true},
		{"r", i18n.T("Recharger", "Reload"), true},
	}
}

func (m *Model) scrollComments(delta int) {
	lines, _ := m.commentsLayout(m.width)
	m.cscroll = max(0, min(m.cscroll+delta, len(lines)-m.bodyHeight()))
}

func (m Model) commentsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "left":
		m.view = ""
	case "up", "k":
		m.scrollComments(-1)
	case "down", "j":
		m.scrollComments(1)
	case "pgup", "u":
		m.scrollComments(-m.bodyHeight() / 2)
	case "pgdown", "d", " ":
		m.scrollComments(m.bodyHeight() / 2)
	case "i":
		return m.threadImages(0)
	case "b":
		m.showBots = !m.showBots
	case "o":
		url, _ := m.prURLOf()
		return m, func() tea.Msg { return doneMsg{err: exec.Command("open", url).Run()} }
	case "r":
		url, _ := m.prURLOf()
		m.thread = nil
		return m, loadThread(m.threadKey, url)
	}
	return m, nil
}

func (m Model) commentsMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch ev.Button {
	case tea.MouseButtonWheelUp:
		m.scrollComments(-3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.scrollComments(3)
		return m, nil
	}
	if ev.Action == tea.MouseActionMotion {
		_, m.hoverBtn = m.hitTest(ev.X, ev.Y)
		return m, nil
	}
	if ev.Button != tea.MouseButtonLeft || ev.Action != tea.MouseActionPress || m.busy != "" {
		return m, nil
	}
	bodyH := m.bodyHeight()
	switch {
	case ev.Y == 0 && ev.X < lipgloss.Width(backLabel())+1:
		m.view = ""
	case ev.Y == 2+bodyH:
		return m.footerClick(ev.X)
	case ev.Y >= 2 && ev.Y < 2+bodyH:
		lines, targets := m.commentsLayout(m.width)
		row := ev.Y - 2 + min(m.cscroll, max(0, len(lines)-bodyH))
		if row < len(targets) && targets[row] > 0 {
			return m.threadImages(targets[row] - 1)
		}
		if row < len(targets) && targets[row] < 0 {
			plain := []rune(xansi.Strip(lines[row]))
			at := len([]rune(strings.Split(string(plain), sendLabel)[0]))
			if ev.X >= at && ev.X < at+len([]rune(sendLabel)) {
				return m.sendComment(-targets[row] - 1)
			}
		}
	}
	return m, nil
}

var (
	mdMu        sync.Mutex
	mdRenderers = map[int]*glamour.TermRenderer{}
	mdCache     = map[string][]string{}
)

// mdStyle is Dracula without the raw-looking bits: headings lose their "##",
// task boxes become cairn's ✓/○, and URLs fade behind their text.
func mdStyle() ansi.StyleConfig {
	cfg := styles.DraculaStyleConfig
	str := func(v string) *string { return &v }
	yes := true
	heading := func(b ansi.StyleBlock, color string) ansi.StyleBlock {
		b.Prefix, b.Suffix = "", ""
		b.Color, b.Bold = str(color), &yes
		return b
	}
	cfg.H1 = heading(cfg.H1, "#ff79c6")
	cfg.H1.BackgroundColor = nil
	cfg.H1.Underline = &yes
	cfg.H2 = heading(cfg.H2, "#bd93f9")
	cfg.H2.Underline = &yes
	cfg.H3 = heading(cfg.H3, "#bd93f9")
	cfg.H4 = heading(cfg.H4, "#8be9fd")
	cfg.H5 = heading(cfg.H5, "#8be9fd")
	cfg.H6 = heading(cfg.H6, "#6272a4")
	cfg.Task.Ticked, cfg.Task.Unticked = "✓ ", "○ "
	cfg.Link.Color, cfg.Link.Underline = str("#6272a4"), nil
	return cfg
}

// renderMarkdown renders a comment as styled terminal lines, wrapped to w.
// Rendering is cached: the view redraws on every mouse move.
func renderMarkdown(body string, w int) []string {
	mdMu.Lock()
	defer mdMu.Unlock()
	key := strconv.Itoa(w) + "\x00" + body
	if lines, ok := mdCache[key]; ok {
		return lines
	}
	r, ok := mdRenderers[w]
	if !ok {
		var err error
		r, err = glamour.NewTermRenderer(glamour.WithStyles(mdStyle()), glamour.WithWordWrap(max(20, w-4)))
		if err != nil {
			return wrap(body, w-2, "  ")
		}
		mdRenderers[w] = r
	}
	out, err := r.Render(body)
	if err != nil {
		return wrap(body, w-2, "  ")
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(xansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	mdCache[key] = lines
	return lines
}

// proofImages lists the image files cited in the selected chantier's proofs.
func (m Model) proofImages() []string {
	c := m.selected()
	if c == nil || c.State == nil {
		return nil
	}
	var out []string
	for _, p := range c.State.Proofs {
		if f := imagePath(p); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// imagePath finds a local image path in a proof line.
func imagePath(s string) string {
	for _, f := range strings.Fields(strings.NewReplacer("`", " ", "(", " ", ")", " ").Replace(s)) {
		low := strings.ToLower(f)
		if (strings.HasPrefix(f, "/") || strings.HasPrefix(f, "~/")) &&
			(strings.HasSuffix(low, ".png") || strings.HasSuffix(low, ".jpg") || strings.HasSuffix(low, ".jpeg") ||
				strings.HasSuffix(low, ".gif") || strings.HasSuffix(low, ".webp")) {
			return f
		}
	}
	return ""
}
