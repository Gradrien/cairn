package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
		m.say("Pas de PR pour ce chantier.", nil)
		return m, nil
	}
	key := m.selected().Key
	m.view, m.cscroll, m.showBots = "comments", 0, false
	if m.thread != nil && m.threadKey == key {
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
			return imagesMsg{err: fmt.Errorf("aucune image téléchargée (%v)", errs[0])}
		}
		msg := imagesMsg{files: okFiles, captions: okCaps, start: newStart}
		if failed > 0 {
			msg.err = fmt.Errorf("%d image(s) inaccessibles", failed)
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
		m.say("Pas d'image dans cette PR.", nil)
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
		m.say("Toutes les images de cette PR sont des liens cassés.", nil)
		return m, nil
	}
	m.busy = fmt.Sprintf("Téléchargement de %d image(s)…", len(srcs))
	return m, openImages(srcs, caps, images.Source{Owner: t.Owner, Repo: t.Repo, Ref: t.HeadRef}, newStart)
}

// commentsLayout renders the conversation; targets give the image (1-based)
// a line opens, 0 for none.
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
		add(0, sMuted.Render("Chargement de la conversation…"))
		return lines, targets
	}
	for _, e := range t.Entries {
		head := sBold.Render(e.Author)
		if e.Bot {
			head = sMuted.Render(e.Author + " (bot)")
		}
		kind := e.Kind
		switch e.State {
		case "APPROVED":
			kind = sGood.Render("review approuvée")
		case "CHANGES_REQUESTED":
			kind = sErr.Render("changements demandés")
		}
		if e.Kind == "inline" {
			kind = sMuted.Render("inline ") + lipgloss.NewStyle().Foreground(working).Render(fmt.Sprintf("%s:%d", e.Path, e.Line))
		}
		add(0, head+sMuted.Render(" · ")+kind+sMuted.Render(" · "+progress.Ago(e.At)))
		body := renderMarkdown(e.Body, w)
		if e.Bot && !m.showBots && len(body) > 8 {
			hidden := len(body) - 8
			body = append(body[:8], sMuted.Render(fmt.Sprintf("  … %d lignes de bot masquées (bouton Bots)", hidden)))
		}
		for _, l := range body {
			img := 0
			if mm := prthread.ImageRe.FindStringSubmatch(ansi.Strip(l)); mm != nil {
				img, _ = strconv.Atoi(mm[1])
				// glamour splits "[image N]" into several styled runs: restyle
				// the line from plain text so the marker reads as one link.
				l = prthread.ImageRe.ReplaceAllStringFunc(strings.TrimRight(ansi.Strip(l), " "), func(s string) string {
					n, _ := strconv.Atoi(prthread.ImageRe.FindStringSubmatch(s)[1])
					if t.Broken[n] {
						return sErr.Render(fmt.Sprintf("[img %d : lien cassé %s]", n, t.Images[n-1]))
					}
					return sImg.Render(fmt.Sprintf("[img %d]", n))
				})
				if t.Broken[img] {
					img = 0
				}
			} else if e.Bot {
				l = sMuted.Render(ansi.Strip(l))
			}
			add(img, l)
		}
		add(0, "")
	}
	return lines, targets
}

func (m Model) commentsView() string {
	_, num := m.prURLOf()
	head := sKey.Render("← Retour") + "   " + sTitle.Render(fmt.Sprintf("PR #%d", num))
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
		head += sMuted.Render(fmt.Sprintf(" · %d commentaires · %d images", n, len(t.Images)-len(t.Broken)))
		if len(t.Broken) > 0 {
			head += sErr.Render(fmt.Sprintf(" · %d liens d'image cassés", len(t.Broken)))
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
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	body := lipgloss.NewStyle().Width(m.width).Height(bodyH).MaxHeight(bodyH).Render(strings.Join(lines, "\n"))
	return ansi.Truncate(head, m.width, "…") + "\n" + m.viewFlash() + "\n" + body + "\n" + m.viewFooter()
}

func (m Model) commentsButtons() []button {
	n := 0
	if m.thread != nil {
		n = len(m.thread.Images) - len(m.thread.Broken)
	}
	bots := "Bots"
	if m.showBots {
		bots = "Masquer bots"
	}
	return []button{
		{"esc", "Retour", true},
		{"i", fmt.Sprintf("Images (%d)", n), n > 0},
		{"b", bots, m.thread != nil},
		{"o", "Ouvrir sur GitHub", true},
		{"r", "Recharger", true},
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
	case ev.Y == 0 && ev.X < 9:
		m.view = ""
	case ev.Y == 2+bodyH:
		_, ranges := m.footerLayout()
		for i, b := range m.buttons() {
			if b.enabled && ev.X >= ranges[i][0] && ev.X < ranges[i][1] {
				return m.key(keyFor(b.key))
			}
		}
	case ev.Y >= 2 && ev.Y < 2+bodyH:
		lines, targets := m.commentsLayout(m.width)
		row := ev.Y - 2 + min(m.cscroll, max(0, len(lines)-bodyH))
		if row < len(targets) && targets[row] > 0 {
			return m.threadImages(targets[row] - 1)
		}
	}
	return m, nil
}

var (
	mdMu        sync.Mutex
	mdRenderers = map[int]*glamour.TermRenderer{}
	mdCache     = map[string][]string{}
)

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
		r, err = glamour.NewTermRenderer(glamour.WithStandardStyle("dracula"), glamour.WithWordWrap(max(20, w-4)))
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
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
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
