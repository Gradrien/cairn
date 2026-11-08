package tracker

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// BranchPRs lists your pull requests of the repository's GitHub remote by
// head branch, newest first: they tie a chantier to its PR whatever the
// tracker knows. Nil when the repository is not on GitHub or gh is missing.
func BranchPRs(repo string) map[string][]PullRequest {
	cmd := exec.Command("gh", "pr", "list", "--state", "all", "--author", "@me", "--limit", "100",
		"--json", "url,number,state,isDraft,headRefName")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var prs []struct {
		URL         string `json:"url"`
		Number      int    `json:"number"`
		State       string `json:"state"`
		IsDraft     bool   `json:"isDraft"`
		HeadRefName string `json:"headRefName"`
	}
	if json.Unmarshal(out, &prs) != nil {
		return nil
	}
	res := map[string][]PullRequest{}
	for _, p := range prs {
		state := strings.ToLower(p.State)
		if state == "open" && p.IsDraft {
			state = "draft"
		}
		res[p.HeadRefName] = append(res[p.HeadRefName], PullRequest{URL: p.URL, Number: p.Number, State: state, Branch: p.HeadRefName})
	}
	return res
}
