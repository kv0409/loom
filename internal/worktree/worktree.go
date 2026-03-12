package worktree

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
)

type Worktree struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path"`
	Branch string `yaml:"branch"`
	Agent  string `yaml:"agent"`
	Issue  string `yaml:"issue"`
}

type DiffStats struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

func projectRoot(loomRoot string) string {
	return filepath.Dir(loomRoot)
}

func Create(loomRoot string, issueID string, slug string, agent string) (*Worktree, error) {
	name := issueID + "-" + slug
	branch := issueID + "-" + slug
	wtPath := filepath.Join(".loom", "worktrees", name)
	absPath := filepath.Join(projectRoot(loomRoot), wtPath)

	// Reuse existing worktree for the same issue
	if _, err := os.Stat(absPath); err == nil {
		return &Worktree{
			Name:   name,
			Path:   absPath,
			Branch: branch,
			Agent:  agent,
			Issue:  issueID,
		}, nil
	}

	cmd := exec.Command("git", "worktree", "add", wtPath, "-b", branch)
	cmd.Dir = projectRoot(loomRoot)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}

	return &Worktree{
		Name:   name,
		Path:   absPath,
		Branch: branch,
		Agent:  agent,
		Issue:  issueID,
	}, nil
}

// HasDirtyFiles returns true if the worktree at the given path has uncommitted changes
// (staged or unstaged, including untracked files).
func HasDirtyFiles(wtPath string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// IsMerged checks whether a branch has been integrated into HEAD.
// It first tries git merge-base --is-ancestor (works for regular merges).
// If that fails and the merge strategy is "squash", it falls back to checking
// whether the associated issue has MergedAt set.
func IsMerged(loomRoot, branchName string) bool {
	root := projectRoot(loomRoot)

	// Strategy 1: git merge-base --is-ancestor (works for regular merges).
	cmd := exec.Command("git", "merge-base", "--is-ancestor", branchName, "HEAD")
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		return true
	}

	// Strategy 2: for squash merges, check issue MergedAt.
	cfg, err := config.Load(loomRoot)
	if err != nil || cfg.Merge.Strategy != "squash" {
		return false
	}
	issueID := ExtractIssueID(branchName)
	if issueID == "" {
		return false
	}
	iss, err := issue.Load(loomRoot, issueID)
	if err != nil {
		return false
	}
	return iss.MergedAt != nil
}

// ErrUnmergedBranch is returned when Remove is called on a branch that has not been merged.
var ErrUnmergedBranch = fmt.Errorf("branch has unmerged commits")

// SalvageCommit commits all dirty/untracked files with a [loom-salvage] message,
// preserving them in git history on the feature branch before worktree removal.
func SalvageCommit(wtPath string, agentID string) error {
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = wtPath
	if _, err := addCmd.CombinedOutput(); err != nil {
		return err
	}
	msg := fmt.Sprintf("[loom-salvage] uncommitted work from agent %s", agentID)
	commitCmd := exec.Command("git", "commit", "-m", msg, "--allow-empty")
	commitCmd.Dir = wtPath
	_, err := commitCmd.CombinedOutput()
	return err
}

// isAlreadyGone returns true when the git worktree remove output indicates the
// worktree path simply doesn't exist (i.e. it was already removed).
func isAlreadyGone(output string) bool {
	return strings.Contains(output, "is not a working tree") ||
		strings.Contains(output, "does not exist")
}

// ForceRemove passes --force to git worktree remove. Used as a fallback when
// normal Remove fails (e.g. dirty worktree after SalvageCommit failure).
func ForceRemove(loomRoot string, name string) error {
	root := projectRoot(loomRoot)
	wtPath := filepath.Join(".loom", "worktrees", name)
	absPath := filepath.Join(root, wtPath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil // already gone
	}
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil && !isAlreadyGone(string(out)) {
		return fmt.Errorf("git worktree remove --force: %s", strings.TrimSpace(string(out)))
	}
	cmd = exec.Command("git", "branch", "-D", name)
	cmd.Dir = root
	cmd.CombinedOutput() // ignore error (branch may already be gone)
	return nil
}

// ListForIssue returns all worktrees for a given issue (not just the first match).
func ListForIssue(loomRoot string, issueID string) ([]*Worktree, error) {
	all, err := List(loomRoot)
	if err != nil {
		return nil, err
	}
	var result []*Worktree
	for _, wt := range all {
		if wt.Issue == issueID {
			result = append(result, wt)
		}
	}
	return result, nil
}

// Remove deletes a worktree and its branch. If force is false, it refuses to
// delete a branch that has not been merged into HEAD (checked via git merge-base --is-ancestor).
func Remove(loomRoot string, name string, force bool) error {
	root := projectRoot(loomRoot)

	if !force {
		if !IsMerged(loomRoot, name) {
			return fmt.Errorf("%w: refusing to delete branch %s (use force to override)", ErrUnmergedBranch, name)
		}
	}

	wtPath := filepath.Join(".loom", "worktrees", name)
	absPath := filepath.Join(root, wtPath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil // already gone
	}
	cmd := exec.Command("git", "worktree", "remove", wtPath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil && !isAlreadyGone(string(out)) {
		return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(out)))
	}

	flag := "-d"
	if cfg, err := config.Load(loomRoot); err == nil && cfg.Merge.Strategy == "squash" {
		flag = "-D"
	}
	cmd = exec.Command("git", "branch", flag, name)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch %s: %s", flag, strings.TrimSpace(string(out)))
	}
	return nil
}

func List(loomRoot string) ([]*Worktree, error) {
	root := projectRoot(loomRoot)
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	wtDir := filepath.Join(root, ".loom", "worktrees") + string(filepath.Separator)
	var worktrees []*Worktree
	var current struct{ path, branch string }

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.path = strings.TrimPrefix(line, "worktree ")
			current.branch = ""
		case strings.HasPrefix(line, "branch "):
			current.branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "":
			if strings.HasPrefix(current.path, wtDir) {
				name := filepath.Base(current.path)
				wt := &Worktree{
					Name:   name,
					Path:   current.path,
					Branch: current.branch,
				}
				parseNameConvention(wt)
				worktrees = append(worktrees, wt)
			}
			current.path = ""
			current.branch = ""
		}
	}
	// Handle last entry (no trailing blank line)
	if current.path != "" && strings.HasPrefix(current.path, wtDir) {
		name := filepath.Base(current.path)
		wt := &Worktree{
			Name:   name,
			Path:   current.path,
			Branch: current.branch,
		}
		parseNameConvention(wt)
		worktrees = append(worktrees, wt)
	}

	return worktrees, nil
}

var issueIDRe = regexp.MustCompile(`^(LOOM-\d+(?:-\d+)?)`)

// ExtractIssueID returns the issue ID prefix from a worktree/branch name,
// or empty string if the name doesn't match the convention.
func ExtractIssueID(name string) string {
	if m := issueIDRe.FindStringSubmatch(name); len(m) > 1 {
		return m[1]
	}
	return ""
}

// parseNameConvention extracts agent/issue from naming convention.
// Name format: <issueID>-<slug>
func parseNameConvention(wt *Worktree) {
	wt.Issue = ExtractIssueID(wt.Name)
}

func Show(loomRoot string, name string) (*Worktree, *DiffStats, error) {
	worktrees, err := List(loomRoot)
	if err != nil {
		return nil, nil, err
	}

	var wt *Worktree
	for _, w := range worktrees {
		if w.Name == name {
			wt = w
			break
		}
	}
	if wt == nil {
		return nil, nil, fmt.Errorf("worktree %s not found", name)
	}

	stats, _ := diffStats(wt.Path)
	return wt, stats, nil
}

// DiffStatsFor returns diff stats for a worktree path.
func DiffStatsFor(wtPath string) (*DiffStats, error) { return diffStats(wtPath) }

// DefaultBranch detects the repo's default branch via origin/HEAD, falling back to "main".
func DefaultBranch(wtPath string) string {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err == nil {
		if branch := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")); branch != "" {
			return branch
		}
	}
	return "main"
}

func diffStats(wtPath string) (*DiffStats, error) {
	base := DefaultBranch(wtPath)
	cmd := exec.Command("git", "diff", "--stat", base+"...HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return &DiffStats{}, nil
	}
	return parseDiffStat(string(out)), nil
}

var diffSummaryRe = regexp.MustCompile(`(\d+) files? changed(?:, (\d+) insertions?\(\+\))?(?:, (\d+) deletions?\(-\))?`)

func parseDiffStat(output string) *DiffStats {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return &DiffStats{}
	}
	summary := lines[len(lines)-1]
	m := diffSummaryRe.FindStringSubmatch(summary)
	if m == nil {
		return &DiffStats{}
	}
	ds := &DiffStats{}
	ds.FilesChanged, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		ds.Insertions, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		ds.Deletions, _ = strconv.Atoi(m[3])
	}
	return ds
}

func Cleanup(loomRoot string) ([]string, error) {
	worktrees, err := List(loomRoot)
	if err != nil {
		return nil, err
	}

	agentsDir := filepath.Join(loomRoot, "agents")
	registered := make(map[string]bool)
	entries, err := os.ReadDir(agentsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			registered[strings.TrimSuffix(e.Name(), ".yaml")] = true
		}
	}

	activeStatuses := map[string]bool{
		"open": true, "assigned": true, "in-progress": true,
		"blocked": true, "review": true,
	}

	var orphaned []string
	for _, wt := range worktrees {
		if wt.Agent != "" {
			if !registered[wt.Agent] {
				orphaned = append(orphaned, wt.Name)
			}
			continue
		}
		// Agent is empty — check if the associated issue is still active
		if wt.Issue == "" {
			orphaned = append(orphaned, wt.Name)
			continue
		}
		iss, err := issue.Load(loomRoot, wt.Issue)
		if err != nil || !activeStatuses[iss.Status] {
			orphaned = append(orphaned, wt.Name)
		}
	}

	return orphaned, nil
}
