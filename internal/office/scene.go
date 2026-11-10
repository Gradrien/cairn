package office

import (
	"sort"
	"time"

	"github.com/mattn/go-runewidth"
)

// Kind is what the main agent is doing, which says where it stands.
type Kind int

const (
	Absent     Kind = iota // no agent on the chantier
	Idle                   // at its desk, nothing running
	Thinking               // at its desk, working without a tool yet
	Writing                // at its desk, editing code
	Running                // at its desk, a command
	Reading                // at the shelf
	Testing                // at the test machine
	Planning               // at the whiteboard, writing the plan
	Delegating             // at a colleague's desk, handing over a task
	Waiting                // standing by its desk, waiting for you
)

// Activity is what the main agent does now.
type Activity struct {
	Kind  Kind
	Label string // its speech bubble
	To    string // Delegating: the colleague's ID
}

// Mood of a colleague.
const (
	Working    = iota // at its desk
	Delivering        // bringing its result to the main agent's desk
	Leaving           // walking out
)

// Colleague is a subagent in the room.
type Colleague struct {
	ID, Role string
	Bubble   string
	Mood     int
}

// Line is one line of the whiteboard.
type Line struct {
	Text string
	Done bool // in green
	Hot  bool // being written now
}

// Input is everything the room shows, rebuilt at every frame.
type Input struct {
	Main       Activity
	Colleagues []Colleague
	Board      []Line
	Now        time.Time
}

// Point is a pixel position: a person's top-left corner, or a thing's.
type Point struct{ X, Y int }

// Layout places the furniture in a room of W×H pixels.
type Layout struct {
	W, H     int
	Big      bool
	WallH    int
	Board    Point
	BoardW   int
	BoardH   int
	Shelf    *Point
	Window   *Point
	Machine  *Point
	Door     *Point
	Plant    Point
	MainDesk Point
	Desks    []Point // the colleagues' desks
	Exit     Point   // where colleagues come in and leave
}

// NewLayout lays out a room drawn on cols×rows cells.
func NewLayout(cols, rows int) Layout {
	l := Layout{W: cols, H: 2 * rows, Big: cols >= 70 && rows >= 20}
	if l.Big {
		l.WallH = min(32, max(20, l.H*36/100))
		l.Board, l.BoardW, l.BoardH = Point{3, 2}, min(52, l.W*40/100), l.WallH-4
		sh := Point{l.Board.X + l.BoardW + 4, l.WallH - 11}
		if sh.X+16 < l.W-30 {
			l.Shelf = &sh
		}
		d := Point{l.W - 30, l.WallH - 16}
		l.Door = &d
		if wx := (l.Board.X + l.BoardW + 4 + 16 + 6); l.Shelf != nil && wx+13 < d.X-4 {
			w := Point{wx, 4}
			l.Window = &w
		}
		mc := Point{l.W - 13, l.WallH - 6}
		l.Machine = &mc
		l.Plant = Point{l.W - 19, l.WallH - 2}
		floorH := l.H - l.WallH
		l.MainDesk = Point{max(4, l.W*12/100), l.WallH + floorH*40/100}
		y := l.WallH + floorH*55/100
		for x := l.W * 42 / 100; x+deskW < l.W-2 && len(l.Desks) < 3; x += deskW + 8 {
			l.Desks = append(l.Desks, Point{x, y})
		}
		l.Exit = Point{d.X + 2, l.WallH - 6}
		return l
	}
	l.WallH = min(8, max(5, l.H*38/100))
	l.Board, l.BoardW, l.BoardH = Point{2, 1}, min(12, l.W/4), l.WallH-2
	l.Plant = Point{l.W - 6, l.WallH - 3}
	l.MainDesk = Point{3, l.WallH + 2}
	for x := l.W * 40 / 100; x+deskW < l.W-6 && len(l.Desks) < 2; x += deskW + 3 {
		l.Desks = append(l.Desks, Point{x, l.WallH + 2})
	}
	l.Exit = Point{l.W + 2, l.WallH + 1}
	return l
}

// seat is where someone sits at desk d: behind the table, head above it.
func seat(d Point) Point    { return Point{d.X + 7, d.Y} }
func beside(d Point) Point  { return Point{d.X + deskW + 1, d.Y} }
func inFront(d Point) Point { return Point{d.X - 7, d.Y} }

// person is someone in the room, moving towards its target.
type person struct {
	id, role string
	at, to   Point
	steps    int
	sitting  bool // sits once at its seat
	ask      bool
	bubble   string
	warn     bool
	leaving  bool
}

// Room keeps the people of one chantier's office between frames.
type Room struct {
	w, h   int // the layout the positions belong to
	people map[string]*person
	slots  map[string]int // colleague → desk
	frame  int
}

func NewRoom() *Room { return &Room{people: map[string]*person{}, slots: map[string]int{}} }

// Step moves everyone one step towards where in sends them. With a new
// layout, everyone is put straight where they go.
func (r *Room) Step(in Input, l Layout) {
	jump := r.w != l.W || r.h != l.H
	r.w, r.h = l.W, l.H
	r.frame++
	r.aim(in, l, jump)
	for id, p := range r.people {
		if jump {
			p.at = p.to
		}
		moved := move(p)
		if moved {
			p.steps++
		}
		if p.leaving && p.at == p.to {
			delete(r.people, id)
			delete(r.slots, id)
		}
	}
}

// move walks p a few pixels: sideways first, then up or down.
func move(p *person) bool {
	dx, dy := p.to.X-p.at.X, p.to.Y-p.at.Y
	switch {
	case dx != 0:
		p.at.X += clamp(dx, 3)
	case dy != 0:
		p.at.Y += clamp(dy, 2)
	default:
		return false
	}
	return true
}

func clamp(d, n int) int {
	switch {
	case d > n:
		return n
	case d < -n:
		return -n
	}
	return d
}

// aim sets every person's target from in.
func (r *Room) aim(in Input, l Layout, jump bool) {
	// Colleagues: a desk while they work, the main desk to deliver, the door after.
	want := map[string]bool{}
	for _, c := range in.Colleagues {
		want[c.ID] = true
		p := r.people[c.ID]
		if p == nil {
			if c.Mood == Leaving {
				continue // gone before we saw it
			}
			p = &person{id: c.ID, role: c.Role, at: l.Exit}
			r.people[c.ID] = p
		}
		p.bubble, p.leaving, p.sitting = c.Bubble, false, false
		switch c.Mood {
		case Working:
			if _, ok := r.slots[c.ID]; !ok {
				r.slots[c.ID] = r.freeSlot(len(l.Desks))
			}
			if s := r.slots[c.ID]; s >= 0 {
				p.to, p.sitting = seat(l.Desks[s]), true
			} else {
				p.to = Point{l.W*70/100 + 8*(len(r.people)%3), l.H - 10}
			}
		case Delivering:
			delete(r.slots, c.ID)
			p.to = beside(l.MainDesk)
		default:
			delete(r.slots, c.ID)
			p.to, p.leaving, p.bubble = l.Exit, true, ""
		}
	}
	for id, p := range r.people {
		if id != "main" && !want[id] {
			delete(r.slots, id)
			p.to, p.leaving, p.bubble = l.Exit, true, ""
		}
	}

	// The main agent.
	if in.Main.Kind == Absent {
		delete(r.people, "main")
		return
	}
	m := r.people["main"]
	if m == nil {
		m = &person{id: "main", role: "main", at: seat(l.MainDesk)}
		r.people["main"] = m
	}
	m.bubble, m.warn, m.ask, m.sitting = in.Main.Label, false, false, false
	m.to = seat(l.MainDesk)
	switch in.Main.Kind {
	case Planning:
		m.to = Point{l.Board.X + l.BoardW/2 - 3, l.WallH - 4}
	case Reading:
		if l.Shelf != nil {
			m.to = Point{l.Shelf.X + 5, l.WallH - 4}
		} else {
			m.sitting = true
		}
	case Testing:
		if l.Machine != nil {
			m.to = Point{l.Machine.X - 7, l.WallH - 3}
		} else {
			m.sitting = true
		}
	case Delegating:
		if s, ok := r.slots[in.Main.To]; ok && s >= 0 {
			m.to = beside(l.Desks[s])
		} else {
			m.sitting = true
		}
	case Waiting:
		m.to, m.warn, m.ask = inFront(l.MainDesk), true, true
		if m.to.X < 1 {
			m.to = beside(l.MainDesk) // no room on the left: wait on the right
		}
	default:
		m.sitting = true
	}
	if jump {
		m.at = m.to
	}
}

func (r *Room) freeSlot(n int) int {
	used := map[int]bool{}
	for _, s := range r.slots {
		used[s] = true
	}
	for i := 0; i < n; i++ {
		if !used[i] {
			return i
		}
	}
	return -1
}

// Options say what a room shows besides the people.
type Options struct {
	Bubbles bool // speech bubbles
	Text    bool // the whiteboard's text
}

// Draw paints the room on c. A room never stepped is set up first.
func (r *Room) Draw(c *Canvas, in Input, l Layout, o Options) {
	if r.w != l.W || r.h != l.H {
		r.Step(in, l)
	}
	// Walls and floor.
	c.Rect(0, 0, l.W, l.WallH, wall)
	for x := 7; x < l.W; x += 18 {
		c.Rect(x, 0, 1, l.WallH-1, wallLine)
	}
	c.Rect(0, l.WallH-1, l.W, 1, baseboard)
	c.Rect(0, l.WallH, l.W, l.H-l.WallH, floor)
	for y := l.WallH + 3; y < l.H; y += 4 {
		c.Rect(0, y, l.W, 1, floorLine)
	}

	// On the wall.
	c.Rect(l.Board.X, l.Board.Y, l.BoardW, l.BoardH, "#9aa0a6")
	c.Rect(l.Board.X+1, l.Board.Y+1, l.BoardW-2, l.BoardH-2, paper)
	if !o.Text {
		for i, ln := range in.Board {
			y := l.Board.Y + 2 + 2*i
			if y >= l.Board.Y+l.BoardH-1 {
				break
			}
			col := Color("#9ca3af")
			switch {
			case ln.Hot:
				col = "#bd93f9"
			case ln.Done:
				col = "#4f9a6a"
			}
			c.Rect(l.Board.X+2, y, min(l.BoardW-4, 3+len(ln.Text)/3), 1, col)
		}
	}
	if l.Shelf != nil {
		c.Sprite(l.Shelf.X, l.Shelf.Y, shelf, shelfPal)
	}
	if l.Window != nil {
		c.Sprite(l.Window.X, l.Window.Y, window, windowPal)
	}
	if l.Door != nil {
		c.Sprite(l.Door.X, l.Door.Y, door, doorPal)
	}
	if l.Machine != nil {
		c.Sprite(l.Machine.X, l.Machine.Y, machine, machinePal)
		testing := in.Main.Kind == Testing
		for i, x := range []int{1, 3, 5} {
			col := ledOff
			switch {
			case testing && (r.frame/2+i)%3 == 0:
				col = ledAmber
			case testing:
				col = ledOn
			case i == 0:
				col = ledOn
			}
			c.Px(l.Machine.X+x, l.Machine.Y+1, col)
		}
	}
	c.Sprite(l.Plant.X, l.Plant.Y, plant, plantPal)

	// Desks and people, back to front.
	type thing struct {
		bottom int
		draw   func()
	}
	var things []thing
	seated := map[Point]bool{}
	for _, p := range r.people {
		if p.sitting && p.at == p.to {
			seated[Point{p.to.X - 7, p.to.Y}] = true
		}
	}
	desks := append([]Point{l.MainDesk}, l.Desks...)
	for i, d := range desks {
		d, lit := d, seated[d] || (i == 0 && in.Main.Kind != Absent && in.Main.Kind != Idle)
		things = append(things, thing{d.Y + deskH, func() {
			if !seated[d] {
				c.Sprite(d.X+7, d.Y+1, chair, chairPal)
			}
			c.Sprite(d.X, d.Y, desk, deskPal)
			if lit {
				c.Px(d.X+3, d.Y+1, ledOn)
			}
		}})
	}
	for _, p := range r.people {
		p := p
		s := personA
		y := p.at.Y
		switch {
		case p.ask:
			s = personAsk
		case p.at != p.to && p.steps%2 == 1:
			s, y = personB, y-1
		}
		things = append(things, thing{p.at.Y + 7, func() { c.Sprite(p.at.X, y, s, lookOf(p.role).pal()) }})
	}
	sort.SliceStable(things, func(i, j int) bool { return things[i].bottom < things[j].bottom })
	for _, t := range things {
		t.draw()
	}

	// Text: the whiteboard, then the bubbles over everything.
	if o.Text {
		row := (l.Board.Y + 3) / 2
		w := l.BoardW - 4
		for _, ln := range in.Board {
			if 2*row >= l.Board.Y+l.BoardH-2 {
				break
			}
			fg := ink
			switch {
			case ln.Hot:
				fg = "#7c3aed"
			case ln.Done:
				fg = "#15803d"
			}
			c.Text(l.Board.X+2, row, cut(ln.Text, w), fg, paper, ln.Hot)
			row++
		}
	}
	if o.Bubbles {
		ids := make([]string, 0, len(r.people))
		for id := range r.people {
			ids = append(ids, id)
		}
		sort.Strings(ids) // stable drawing
		taken := map[int][][2]int{}
		free := func(row, a, b int) bool {
			for _, s := range taken[row] {
				if a < s[1] && s[0] < b {
					return false
				}
			}
			return true
		}
		for _, id := range ids {
			p := r.people[id]
			if p.bubble == "" {
				continue
			}
			text := " " + cut(p.bubble, 34) + " "
			col := p.at.X + 3 - runewidth.StringWidth(text)/2
			col = max(0, min(col, l.W-runewidth.StringWidth(text)))
			row := p.at.Y/2 - 1
			w := runewidth.StringWidth(text)
			for row > 0 && !free(row, col, col+w) {
				row--
			}
			taken[row] = append(taken[row], [2]int{col, col + w})
			bg, fg := bubbleBg, ink
			if p.warn {
				bg = warnBg
			}
			if p.bubble == "z" || p.bubble == "z z" {
				fg = dimInk
			}
			c.Text(col, row, text, fg, bg, p.warn)
		}
	}
}

func cut(s string, w int) string {
	if w <= 1 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}
