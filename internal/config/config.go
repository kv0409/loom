package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/karanagi/loom/internal/store"
	"gopkg.in/yaml.v3"
)

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// SanitizeBasename returns a lowercased, tmux-safe version of the directory basename.
func SanitizeBasename(absDir string) string {
	name := strings.ToLower(unsafeChars.ReplaceAllString(filepath.Base(absDir), "-"))
	name = strings.Trim(name, "-")
	if name == "" {
		name = "default"
	}
	return name
}

// DeriveSessionName returns "loom-<sanitized-basename>-<short-hash>" for the given absolute directory path.
func DeriveSessionName(absDir string) string {
	h := sha256.Sum256([]byte(absDir))
	return "loom-" + SanitizeBasename(absDir) + "-" + hex.EncodeToString(h[:])[:8]
}

type Config struct {
	Project string        `yaml:"project"`
	Limits  LimitsConfig  `yaml:"limits"`
	Merge   MergeConfig   `yaml:"merge"`
	Polling PollingConfig `yaml:"polling"`
	Tmux    TmuxConfig    `yaml:"tmux"`
	Kiro    KiroConfig    `yaml:"kiro"`
	Models  ModelsConfig  `yaml:"models"`
	MCP     MCPConfig     `yaml:"mcp"`
	Deny    DenyConfig    `yaml:"deny"`
}

type ModelsConfig struct {
	Orchestrator string `yaml:"orchestrator"`
	Lead         string `yaml:"lead"`
	Builder      string `yaml:"builder"`
	Reviewer     string `yaml:"reviewer"`
	Explorer     string `yaml:"explorer"`
	Researcher   string `yaml:"researcher"`
}

// ForRole returns the configured model for a given agent role, or "" if unset.
func (m *ModelsConfig) ForRole(role string) string {
	switch role {
	case "orchestrator":
		return m.Orchestrator
	case "lead":
		return m.Lead
	case "builder":
		return m.Builder
	case "reviewer":
		return m.Reviewer
	case "explorer":
		return m.Explorer
	case "researcher":
		return m.Researcher
	default:
		return ""
	}
}

type DenyConfig struct {
	Tools    []string `yaml:"tools"`
	Commands []string `yaml:"commands"`
}

// IsDenied returns true if the given tool name or command matches the deny list.
// Tools are matched by exact name. Commands are matched as glob patterns.
func (d *DenyConfig) IsDenied(tool string, command string) bool {
	for _, t := range d.Tools {
		if t == tool {
			return true
		}
	}
	for _, pattern := range d.Commands {
		if matched, _ := filepath.Match(pattern, command); matched {
			return true
		}
		// Also check if the command starts with the pattern (prefix match for commands with args)
		if strings.Contains(command, " ") {
			parts := strings.SplitN(command, " ", 2)
			if matched, _ := filepath.Match(pattern, parts[0]); matched {
				return true
			}
		}
	}
	return false
}

type LimitsConfig struct {
	MaxAgents               int `yaml:"max_agents"`
	MaxWorktrees            int `yaml:"max_worktrees"`
	MaxAgentsPerLead        int `yaml:"max_agents_per_lead"`
	HeartbeatTimeoutSeconds int `yaml:"heartbeat_timeout_seconds"`
	IdleTimeoutSeconds      int `yaml:"idle_timeout_seconds"`
}

type MergeConfig struct {
	Strategy         string `yaml:"strategy"`
	AutoDeleteBranch bool   `yaml:"auto_delete_branch"`
	RequireReview    bool   `yaml:"require_review"`
}

type PollingConfig struct {
	IssueIntervalMs          int `yaml:"issue_interval_ms"`
	MailIntervalMs           int `yaml:"mail_interval_ms"`
	HeartbeatIntervalMs      int `yaml:"heartbeat_interval_ms"`
	PendingAgentsIntervalMs  int `yaml:"pending_agents_interval_ms"`
	ACPOutputIntervalMs      int `yaml:"acp_output_interval_ms"`
	WorktreeGCIntervalMs     int `yaml:"worktree_gc_interval_ms"`
}

type TmuxConfig struct {
	SessionName      string `yaml:"session_name"`
	OrchestratorWindow int  `yaml:"orchestrator_window"`
	DashboardWindow  int    `yaml:"dashboard_window"`
	AgentWindowStart int    `yaml:"agent_window_start"`
}

type KiroConfig struct {
	Command     string `yaml:"command"`
	DefaultMode string `yaml:"default_mode"`
}

type MCPConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

func DefaultConfig() *Config {
	return &Config{
		Project: "",
		Limits: LimitsConfig{
			MaxAgents:               8,
			MaxWorktrees:            4,
			MaxAgentsPerLead:        3,
			HeartbeatTimeoutSeconds: 300,
			IdleTimeoutSeconds:      600,
		},
		Merge: MergeConfig{
			Strategy:         "squash",
			AutoDeleteBranch: true,
			RequireReview:    true,
		},
		Polling: PollingConfig{
			IssueIntervalMs:         2000,
			MailIntervalMs:          2000,
			HeartbeatIntervalMs:     30000,
			PendingAgentsIntervalMs: 2000,
			ACPOutputIntervalMs:     1000,
			WorktreeGCIntervalMs:    300000,
		},
		Tmux: TmuxConfig{
			SessionName:        "loom",
			OrchestratorWindow: 0,
			DashboardWindow:    1,
			AgentWindowStart:   2,
		},
		Kiro: KiroConfig{
			Command:     "kiro-cli",
			DefaultMode: "acp",
		},
		Models: ModelsConfig{
			Orchestrator: "sonnet",
			Lead:         "sonnet",
			Builder:      "sonnet",
			Reviewer:     "opus",
			Explorer:     "haiku",
			Researcher:   "haiku",
		},
		MCP: MCPConfig{
			Enabled: true,
			Port:    0,
		},
		Deny: DenyConfig{
			Tools: nil,
			Commands: []string{
				"git merge*",
				"git branch -[dD]*",
				"git branch --delete*",
				"git worktree remove*",
				"git worktree prune*",
				"git checkout main*",
				"git checkout master*",
				"git switch main*",
				"git switch master*",
				"git push*",
				"git reset --hard*",
				"git clean -fdx*",
			},
		},
	}
}

func (c *Config) Validate() error {
	switch c.Kiro.DefaultMode {
	case "chat", "acp":
		return nil
	default:
		return fmt.Errorf("invalid kiro.default_mode %q: must be chat or acp", c.Kiro.DefaultMode)
	}
}

func Load(loomRoot string) (*Config, error) {
	cfg := &Config{}
	if err := store.ReadYAML(filepath.Join(loomRoot, "config.yaml"), cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// Backfill defaults for sections missing from existing config files.
	defaults := DefaultConfig()
	if cfg.Models == (ModelsConfig{}) {
		cfg.Models = defaults.Models
	}
	return cfg, nil
}

func Save(loomRoot string, cfg *Config) error {
	return store.WriteYAML(filepath.Join(loomRoot, "config.yaml"), cfg)
}

func Set(loomRoot string, key string, value string) error {
	cfg, err := Load(loomRoot)
	if err != nil {
		return err
	}

	// Marshal to map, set key, unmarshal back
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return err
	}

	if err := setDottedKey(m, key, value); err != nil {
		return err
	}

	data, err = yaml.Marshal(m)
	if err != nil {
		return err
	}
	var updated Config
	if err := yaml.Unmarshal(data, &updated); err != nil {
		return err
	}
	return Save(loomRoot, &updated)
}

func setDottedKey(m map[string]interface{}, key string, value string) error {
	parts := splitDotted(key)
	current := m
	for i, p := range parts {
		if i == len(parts)-1 {
			// Set the value, inferring type from existing value
			existing, exists := current[p]
			if exists {
				switch existing.(type) {
				case int:
					n, err := strconv.Atoi(value)
					if err != nil {
						return fmt.Errorf("expected integer for %s: %w", key, err)
					}
					current[p] = n
				case bool:
					b, err := strconv.ParseBool(value)
					if err != nil {
						return fmt.Errorf("expected bool for %s: %w", key, err)
					}
					current[p] = b
				default:
					current[p] = value
				}
			} else {
				current[p] = value
			}
			return nil
		}
		next, ok := current[p]
		if !ok {
			return fmt.Errorf("unknown config key: %s", key)
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("key %s is not a section", p)
		}
		current = nextMap
	}
	return fmt.Errorf("empty key")
}

func splitDotted(key string) []string {
	var parts []string
	current := ""
	for _, c := range key {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func FindLoomRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ".loom")); err == nil && info.IsDir() {
			return filepath.Join(dir, ".loom"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a loom project (no .loom/ directory found)")
		}
		dir = parent
	}
}
