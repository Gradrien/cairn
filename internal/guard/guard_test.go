package guard

import "testing"

func TestPatterns(t *testing.T) {
	for cmd, want := range map[string]bool{
		"git push":                             true,
		"git push -u origin HEAD":              true,
		"cd /x && git push --force-with-lease": true,
		"git -C /x push":                       true,
		"gh pr create --fill":                  true,
		"gh pr ready 12":                       true,
		"git status && git log":                false,
		"echo 'do not git pushover'":           false,
		"gh pr view 12":                        false,
		"git commit -m 'push button'":          false,
	} {
		got := gitPush.MatchString(cmd) || ghPR.MatchString(cmd)
		if got != want {
			t.Errorf("%q: got %v want %v", cmd, got, want)
		}
	}
}
