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

func Remove(loomRoot string, name string) error {
	root := projectRoot(loomRoot)
	wtPath := filepath.Join(".loom", "worktrees", name)

	cmd := exec.Command("git", "worktree", "remove", wtPath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(out)))
	}

	cmd = exec.Command("git", "branch", "-d", name)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -d: %s", strings.TrimSpace(string(out)))
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

// parseNameConvention extracts agent/issue from naming convention.
// Name format: <issueID>-<slug>
func parseNameConvention(wt *Worktree) {
	re := regexp.MustCompile(`^(LOOM-\d+(?:-\d+)?)`)
	if m := re.FindStringSubmatch(wt.Name); len(m) > 1 {
		wt.Issue = m[1]
	}
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

func diffStats(wtPath string) (*DiffStats, error) {
	cmd := exec.Command("git", "diff", "--stat", "HEAD")
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

	var orphaned []string
	for _, wt := range worktrees {
		if wt.Agent != "" && !registered[wt.Agent] {
			orphaned = append(orphaned, wt.Name)
		}
	}

	// If no agents are registered at all, all worktrees are orphaned
	if len(registered) == 0 && len(worktrees) > 0 {
		for _, wt := range worktrees {
			orphaned = append(orphaned, wt.Name)
		}
	}

	return orphaned, nil
}
