package native

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTake(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	for _, r := range []Request{{Key: "YAK-1", Action: "select"}, {Key: "YAK-2", Action: "select"}, {Key: "YAK-2", Action: "key", Arg: "a"}, {Key: "YAK-3", Action: "select"}} {
		if err := Send(r); err != nil {
			t.Fatal(err)
		}
	}
	old, _ := json.Marshal(Request{Key: "YAK-9", Action: "key", Arg: "x", At: time.Now().Add(-time.Minute).UnixMilli()})
	if err := os.WriteFile(filepath.Join(RequestsDir(), "00000000000000000001-1.json"), old, 0o644); err != nil {
		t.Fatal(err)
	}
	got := Take()
	if len(got) != 3 || got[0].Key != "YAK-2" || got[1].Arg != "a" || got[2].Key != "YAK-3" {
		t.Fatalf("a stale request is dropped and selections in a row keep the last: %+v", got)
	}
	if left, _ := os.ReadDir(RequestsDir()); len(left) != 0 {
		t.Errorf("everything taken is removed, %d left", len(left))
	}
}

func TestLock(t *testing.T) {
	t.Setenv("CAIRN_HOME", t.TempDir())
	if Running() {
		t.Fatal("no engine yet")
	}
	f, err := Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(); err == nil || !Running() {
		t.Error("a second engine is refused while the first runs")
	}
	f.Close()
	if Running() {
		t.Error("the lock goes with the engine")
	}
}

func TestThumb(t *testing.T) {
	app := t.TempDir()
	t.Setenv("HERDR_APP_HOME", app)
	src := filepath.Join(t.TempDir(), "Capture.PNG")
	if err := os.WriteFile(src, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := Thumb(src)
	if !strings.HasPrefix(rel, filepath.Join("mods", "cairn-detail", "proofs")) || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("the copy goes under the detail mod: %q", rel)
	}
	if data, _ := os.ReadFile(filepath.Join(app, rel)); string(data) != "png" {
		t.Error("the image is copied as is")
	}
	if Thumb(src) != rel {
		t.Error("the same file version keeps its copy")
	}
	if Thumb(filepath.Join(t.TempDir(), "gone.png")) != "" {
		t.Error("a missing file has no copy")
	}
}
