// Package tracker is where tickets come from: a ticket tracker (Sprintosaur,
// Linear) or nothing at all (local: the chantier file is the ticket).
package tracker

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"cairn/internal/config"
	"cairn/internal/i18n"
)

type PullRequest struct {
	URL    string `json:"url"`
	Number int    `json:"number"`
	State  string `json:"state"` // open | draft | merged | closed
	Branch string `json:"branch"`
}

type Issue struct {
	ID            string
	Title         string
	Status        string
	Priority      string // "" when none
	PriorityValue int    // 1 most urgent, 0 none
	Estimate      string
	Assigned      bool
	Started       bool // in progress or further: Start leaves it as is
	Done          bool // the tracker says the work is finished
	Cycle         int  // 0 when the tracker has no cycle
	URL           string
	PullRequests  []PullRequest
}

// Tracker reads and moves the tickets of one project.
type Tracker interface {
	// Name is shown on screen: "Sprintosaur", "Linear", "local".
	Name() string
	// Mine lists the open tickets you work on.
	Mine() ([]Issue, error)
	// Free lists open tickets nobody has, offered by "new ticket".
	Free() ([]Issue, error)
	// Get fetches one ticket; its ID is empty when it does not exist.
	Get(key string) (Issue, error)
	// Take assigns the ticket to you.
	Take(key string) error
	// Start moves a ticket not started yet to "in progress"; false when it
	// was already started.
	Start(is Issue) (bool, error)
	// Guide tells a planning agent where to read the ticket.
	Guide(key string) string
}

// NeedsKey is a tracker that has no API key yet: Where says where cairn looked.
type NeedsKey struct{ Tracker, Where string }

func (e *NeedsKey) Error() string {
	return fmt.Sprintf(i18n.T("%s n'a pas de clé d'API : ouvre les réglages (s) pour la coller", "%s has no API key: open the settings (s) to paste it"), e.Tracker)
}

// ErrKeyRefused is an API key the tracker does not accept.
var ErrKeyRefused error = keyRefused{}

// keyRefused translates when shown, not when the package loads.
type keyRefused struct{}

func (keyRefused) Error() string {
	return i18n.T("le tracker refuse la clé d'API : recrée-la et colle-la dans les réglages (s)", "the tracker refuses the API key: create a new one and paste it in the settings (s)")
}

// Creator is a tracker cairn can create tickets in (only from a title).
type Creator interface {
	Create(title string) (Issue, error)
}

// New opens the tracker of a project.
func New(p config.Project, chantiersDir string) (Tracker, error) {
	switch p.Tracker {
	case "", "local":
		return NewLocal(p.Key, chantiersDir), nil
	// A failed constructor's nil pointer must not become a non-nil Tracker.
	case "sprintosaur":
		if s, err := NewSprintosaur(p); err == nil {
			return s, nil
		} else {
			return nil, err
		}
	case "linear":
		if l, err := NewLinear(p); err == nil {
			return l, nil
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf(i18n.T("tracker inconnu %q (local, linear ou sprintosaur)", "unknown tracker %q (local, linear or sprintosaur)"), p.Tracker)
}

// readEnvFile reads a KEY=VALUE file (shell style, optional export/quotes).
func readEnvFile(path string) map[string]string {
	vals := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return vals
	}
	defer f.Close()
	parseEnv(f, vals)
	return vals
}

func parseEnv(f io.Reader, vals map[string]string) {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
}
