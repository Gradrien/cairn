package office

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func busy() Input {
	return Input{
		Main: Activity{Kind: Delegating, Label: "→ cherche les effets de bord", To: "explorer"},
		Colleagues: []Colleague{
			{ID: "explorer", Role: "cairn-explorer", Bubble: "Grep triggerBulk", Mood: Working},
			{ID: "runner", Role: "cairn-runner", Bubble: "✓ 42 tests", Mood: Delivering},
		},
		Board: []Line{{Text: "? 2 questions", Hot: true}, {Text: "Tâches 1/3"}, {Text: "Effets de bord 2/2", Done: true}},
		Now:   time.Now(),
	}
}

func TestLayoutFits(t *testing.T) {
	for _, sz := range [][2]int{{150, 36}, {90, 24}, {46, 9}, {30, 8}} {
		l := NewLayout(sz[0], sz[1])
		for i, d := range append([]Point{l.MainDesk}, l.Desks...) {
			if d.X < 0 || d.X+deskW > l.W || d.Y+deskH > l.H || d.Y-1 < l.WallH-8 {
				t.Errorf("%v: desk %d at %v out of the room %dx%d", sz, i, d, l.W, l.H)
			}
		}
		if l.Big != (sz[0] >= 70) {
			t.Errorf("%v: big %v", sz, l.Big)
		}
	}
}

func TestPeopleWalkThenSit(t *testing.T) {
	l := NewLayout(150, 36)
	r := NewRoom()
	in := Input{Main: Activity{Kind: Writing, Label: "écrit detail.go"}}
	r.Step(in, l)
	if p := r.people["main"]; p.at != seat(l.MainDesk) {
		t.Fatalf("starts at its desk: %v", p.at)
	}
	in.Main = Activity{Kind: Planning, Label: "rédige les questions"}
	for i := 0; i < 200; i++ {
		r.Step(in, l)
	}
	if p := r.people["main"]; p.at != p.to || p.sitting {
		t.Errorf("walked to the whiteboard and stands: %+v", p)
	}
	// A colleague comes in by the door, works at a desk, delivers, then leaves.
	in.Colleagues = []Colleague{{ID: "x", Role: "Explore", Mood: Working}}
	r.Step(in, l)
	if p := r.people["x"]; p == nil || p.at == p.to {
		t.Fatal("a colleague walks in")
	}
	for i := 0; i < 200; i++ {
		r.Step(in, l)
	}
	if p := r.people["x"]; p.at != seat(l.Desks[0]) {
		t.Errorf("at its desk: %v", p.at)
	}
	in.Colleagues[0].Mood = Leaving
	for i := 0; i < 300; i++ {
		r.Step(in, l)
	}
	if _, ok := r.people["x"]; ok {
		t.Error("left the room")
	}
}

func TestWaitingStaysInTheRoom(t *testing.T) {
	for _, sz := range [][2]int{{150, 36}, {46, 9}} {
		l := NewLayout(sz[0], sz[1])
		r := NewRoom()
		r.Step(Input{Main: Activity{Kind: Waiting, Label: "? valider"}}, l)
		if p := r.people["main"]; p.to.X < 0 || p.to.X+6 > l.W {
			t.Errorf("%v: waits at %v, out of the room", sz, p.to)
		}
	}
}

func TestRenderSize(t *testing.T) {
	for _, sz := range [][2]int{{150, 36}, {46, 9}} {
		c := NewCanvas(sz[0], sz[1])
		r := NewRoom()
		in := busy()
		for i := 0; i < 300; i++ {
			r.Step(in, NewLayout(sz[0], sz[1]))
		}
		r.Draw(c, in, NewLayout(sz[0], sz[1]), Options{Bubbles: sz[0] > 70, Text: sz[0] > 70})
		lines := strings.Split(c.Render(), "\n")
		if len(lines) != sz[1] {
			t.Fatalf("%d lines for %d rows", len(lines), sz[1])
		}
		for i, ln := range lines {
			if w := ansi.StringWidth(ln); w != sz[0] {
				t.Errorf("%v line %d is %d wide", sz, i, w)
			}
		}
		if sz[0] > 70 {
			text := ansi.Strip(c.Render())
			for _, want := range []string{"? 2 questions", "Grep triggerBulk", "→ cherche les effets de bord"} {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q", want)
				}
			}
		}
		if dir := os.Getenv("CAIRN_OFFICE"); dir != "" {
			savePNG(t, c, filepath.Join(dir, "office-"+strconv.Itoa(sz[0])+".png"))
		}
	}
}

// savePNG draws the canvas at 6 screen pixels per pixel, to look at it.
func savePNG(t *testing.T, c *Canvas, path string) {
	const s = 6
	img := image.NewRGBA(image.Rect(0, 0, c.Cols*s, c.Rows*2*s))
	hex := func(col Color, def color.RGBA) color.RGBA {
		if len(col) != 7 {
			return def
		}
		v, _ := strconv.ParseUint(string(col[1:]), 16, 32)
		return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
	}
	bg := color.RGBA{40, 42, 54, 255}
	for y, row := range c.cells {
		for x, cl := range row {
			top, bot := hex(cl.top, bg), hex(cl.bot, bg)
			if cl.r != 0 {
				top, bot = hex(cl.bg, bg), hex(cl.bg, bg)
			}
			for dy := 0; dy < 2*s; dy++ {
				for dx := 0; dx < s; dx++ {
					col := top
					if dy >= s {
						col = bot
					}
					if cl.r != 0 && cl.r != ' ' && dy > 3 && dy < 2*s-3 && dx > 0 && dx < s-1 {
						col = hex(cl.fg, color.RGBA{0, 0, 0, 255})
					}
					img.Set(x*s+dx, y*2*s+dy, col)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
