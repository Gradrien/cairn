// Package images downloads pictures (GitHub attachments, signed links, files
// of the PR branch) into a cache, and shows them full screen with the Kitty
// graphics protocol, which herdr passes through to Ghostty and friends.
package images

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"cairn/internal/config"
	"cairn/internal/i18n"
)

// Source says where relative image paths live: a repo at a ref.
type Source struct{ Owner, Repo, Ref string }

func cacheDir() string { return filepath.Join(config.Dir(), "cache", "images") }

func ghToken() string {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Fetch returns a local file for src: a local path, an http(s) URL, or a path
// relative to the PR branch.
func Fetch(src string, from Source) (string, error) {
	if strings.HasPrefix(src, "/") || strings.HasPrefix(src, "~/") {
		if strings.HasPrefix(src, "~/") {
			home, _ := os.UserHomeDir()
			src = filepath.Join(home, src[2:])
		}
		_, err := os.Stat(src)
		return src, err
	}
	sum := sha256.Sum256([]byte(from.Owner + "/" + from.Repo + "@" + from.Ref + ":" + src))
	dst := filepath.Join(cacheDir(), hex.EncodeToString(sum[:12]))
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	if err := os.MkdirAll(cacheDir(), 0o755); err != nil {
		return "", err
	}
	var req *http.Request
	var err error
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		req, err = http.NewRequest(http.MethodGet, src, nil)
		if err != nil {
			return "", err
		}
		if u, _ := url.Parse(src); u != nil && (u.Host == "github.com" || strings.HasSuffix(u.Host, ".githubusercontent.com")) {
			if tok := ghToken(); tok != "" {
				req.Header.Set("Authorization", "token "+tok)
			}
		}
	} else {
		// A file of the PR branch, relative to the repo root.
		p := path.Clean(strings.TrimPrefix(src, "./"))
		api := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", from.Owner, from.Repo, p, url.QueryEscape(from.Ref))
		req, err = http.NewRequest(http.MethodGet, api, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github.raw")
		if tok := ghToken(); tok != "" {
			req.Header.Set("Authorization", "token "+tok)
		}
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 50<<20)); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	return dst, os.Rename(tmp, dst)
}

// pngBytes returns the file as PNG (Kitty's f=100), transcoding other formats.
func pngBytes(file string) ([]byte, image.Point, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, image.Point{}, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, image.Point{}, fmt.Errorf("%s", i18n.T("format d'image non reconnu", "unknown image format"))
	}
	size := image.Pt(cfg.Width, cfg.Height)
	if format == "png" {
		return data, size, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, size, err
	}
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	return buf.Bytes(), size, err
}

const esc = "\x1b"

func kittyPlace(w io.Writer, data []byte, cols, rows int) {
	enc := base64.StdEncoding.EncodeToString(data)
	first := true
	for len(enc) > 0 {
		n := min(4096, len(enc))
		chunk := enc[:n]
		enc = enc[n:]
		more := 0
		if len(enc) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(w, "%s_Ga=T,f=100,q=2,C=1,c=%d,r=%d,m=%d;%s%s\\", esc, cols, rows, more, chunk, esc)
			first = false
		} else {
			fmt.Fprintf(w, "%s_Gm=%d;%s%s\\", esc, more, chunk, esc)
		}
	}
}

// fit returns the cells an image of px pixels takes inside cols×rows cells.
func fit(px image.Point, cols, rows int, cellW, cellH float64) (int, int) {
	wc, hc := float64(px.X)/cellW, float64(px.Y)/cellH
	scale := min(float64(cols)/wc, float64(rows)/hc)
	if scale > 1 {
		scale = 1 // never blow small screenshots up
	}
	return max(1, int(wc*scale)), max(1, int(hc*scale))
}

// Show displays files full screen, one at a time: ←/→ to move, o to open in
// the system viewer, q or Esc to leave. It owns the terminal while running.
func Show(files []string, captions []string, start int) error {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	out := os.Stdout
	fmt.Fprint(out, esc+"[?1049h"+esc+"[?25l")
	defer func() {
		fmt.Fprint(out, esc+"_Ga=d,q=2"+esc+"\\"+esc+"[2J"+esc+"[?25h"+esc+"[?1049l")
		term.Restore(fd, old)
	}()

	i := max(0, min(start, len(files)-1))
	buf := make([]byte, 8)
	for {
		cols, rows, _ := term.GetSize(fd)
		cellW, cellH := 8.0, 16.0
		if ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); err == nil && ws.Xpixel > 0 && ws.Col > 0 {
			cellW, cellH = float64(ws.Xpixel)/float64(ws.Col), float64(ws.Ypixel)/float64(ws.Row)
		}
		fmt.Fprint(out, esc+"_Ga=d,q=2"+esc+"\\"+esc+"[2J"+esc+"[H")
		caption := fmt.Sprintf(" %d/%d  %s", i+1, len(files), captions[i])
		data, px, err := pngBytes(files[i])
		if err != nil {
			fmt.Fprintf(out, i18n.T("\r\n  Impossible d'afficher cette image : %v\r\n", "\r\n  Cannot show this image: %v\r\n"), err)
		} else {
			c, r := fit(px, cols-2, rows-3, cellW, cellH)
			fmt.Fprintf(out, "%s[2;%dH", esc, 1+(cols-c)/2)
			kittyPlace(out, data, c, r)
		}
		help := i18n.T("  ← → image   o ouvrir dans Aperçu   q fermer", "  ← → image   o open in Preview   q close")
		fmt.Fprintf(out, "%s[%d;1H%s[7m%s%s[0m%s", esc, rows, esc, padRight(caption, cols-len([]rune(help))), esc, help)

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}
		k := string(buf[:n])
		switch {
		case k == "q" || k == esc || k == "\x03":
			return nil
		case k == esc+"[C" || k == "l" || k == "n" || k == " ":
			i = (i + 1) % len(files)
		case k == esc+"[D" || k == "h" || k == "p":
			i = (i - 1 + len(files)) % len(files)
		case k == "o":
			opener := "xdg-open"
			if runtime.GOOS == "darwin" {
				opener = "open"
			}
			_ = exec.Command(opener, files[i]).Start()
		}
	}
}

func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:max(0, w-1)]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}
