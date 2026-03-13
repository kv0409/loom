package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/karanagi/loom/agents"
	"github.com/karanagi/loom/internal/agent"
	cliout "github.com/karanagi/loom/internal/cli"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/dashboard"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/lock"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/nudge"
	"github.com/karanagi/loom/internal/mcp"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/tmux"
	"github.com/karanagi/loom/internal/worktree"
	"github.com/karanagi/loom/templates"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var version = "dev"
var commitHash = "unknown"

func main() {
	root := &cobra.Command{
		Use:     "loom",
		Short:   "Multi-agent orchestration for kiro-cli",
		Version: fmt.Sprintf("%s (%s)", version, commitHash),
	}

	// --- loom init ---
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .loom/ in the current git repository",
		RunE:  runInit,
	}
	initCmd.Flags().Bool("force", false, "Overwrite existing .loom/ directory")
	initCmd.Flags().Bool("refresh", false, "Update templates, agents, and hooks without wiping state")

	// --- Lifecycle ---
	lifecycleGroup := &cobra.Group{ID: "lifecycle", Title: "Lifecycle"}
	root.AddGroup(lifecycleGroup)

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Launch orchestrator and daemon",
		RunE:  runStart,
	}
	startCmd.Flags().Bool("resume", false, "Auto-resume without prompting")
	startCmd.Flags().Bool("fresh", false, "Discard previous state")
	startCmd.Flags().String("mode", "", "Kiro mode for orchestrator: chat|acp")
	startCmd.Flags().Bool("no-dashboard", false, "Skip auto-opening the dashboard")
	startCmd.GroupID = "lifecycle"

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Graceful shutdown",
		RunE:  runStop,
	}
	stopCmd.Flags().Bool("force", false, "Send SIGKILL instead of SIGTERM")
	stopCmd.Flags().Bool("daemon-only", false, "Stop only the daemon; leave agents and tmux session running")
	stopCmd.Flags().Bool("clean", false, "Remove all worktrees including unmerged branches")
	stopCmd.GroupID = "lifecycle"

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Hot-reload daemon without killing agents",
		RunE:  runRestart,
	}
	restartCmd.Flags().Bool("no-dashboard", false, "Skip auto-opening the dashboard")
	restartCmd.GroupID = "lifecycle"

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Quick health check",
		RunE:  runStatus,
	}
	statusCmd.GroupID = "lifecycle"

	// --- Dashboard ---
	dashCmd := &cobra.Command{
		Use:   "dash",
		Short: "Launch TUI dashboard",
		RunE:  runDash,
	}

	// --- Task ---
	taskCmd := &cobra.Command{
		Use:   "task <description>",
		Short: "Create a task from natural language",
		Args:  cobra.ExactArgs(1),
		RunE:  runTask,
	}

	// --- Issues ---
	issueCmd := &cobra.Command{Use: "issue", Short: "Issue tracker"}

	issueCreateCmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new issue",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueCreate,
	}
	issueCreateCmd.Flags().String("type", "task", "Issue type: epic|task|bug|spike")
	issueCreateCmd.Flags().String("priority", "normal", "Priority: critical|high|normal|low")
	issueCreateCmd.Flags().String("parent", "", "Parent issue ID")
	issueCreateCmd.Flags().StringP("description", "d", "", "Description")
	issueCreateCmd.Flags().String("depends-on", "", "Comma-separated dependency IDs")

	issueListCmd := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		RunE:  runIssueList,
	}
	issueListCmd.Flags().String("status", "", "Filter by status")
	issueListCmd.Flags().String("assignee", "", "Filter by assignee")
	issueListCmd.Flags().String("type", "", "Filter by type")
	issueListCmd.Flags().Bool("all", false, "Include closed/cancelled")
	issueListCmd.Flags().Bool("tree", false, "Show parent/child hierarchy")

	issueShowCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show issue detail",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueShow,
	}

	issueUpdateCmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an issue",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueUpdate,
	}
	issueUpdateCmd.Flags().String("status", "", "New status")
	issueUpdateCmd.Flags().String("priority", "", "New priority")
	issueUpdateCmd.Flags().String("assignee", "", "New assignee")

	issueCloseCmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close an issue",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueClose,
	}
	issueCloseCmd.Flags().String("reason", "", "Close reason")

	issueCmd.AddCommand(issueCreateCmd, issueListCmd, issueShowCmd, issueUpdateCmd, issueCloseCmd)

	// --- Agents ---
	agentsCmd := &cobra.Command{
		Use:   "agents",
		Short: "List all agents",
		RunE:  runAgents,
	}
	agentCmd := &cobra.Command{Use: "agent", Short: "Agent management"}
	agentShowCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show agent detail",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentShow,
	}
	agentHeartbeatCmd := &cobra.Command{
		Use:    "heartbeat",
		Short:  "Update agent heartbeat",
		Hidden: true,
		RunE:   runAgentHeartbeat,
	}
	agentCancelCmd := &cobra.Command{
		Use:   "cancel <name>",
		Short: "Cancel in-progress ACP prompt",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentCancel,
	}
	agentCmd.AddCommand(agentShowCmd, agentHeartbeatCmd, agentCancelCmd)

	attachCmd := &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach to agent tmux pane",
		Args:  cobra.ExactArgs(1),
		RunE:  runAttach,
	}
	nudgeCmd := &cobra.Command{
		Use:   "nudge <agent> <type>",
		Short: "Send predefined nudge to agent",
		Long: "Send a predefined nudge signal to an agent.\n\nAvailable nudge types:\n" + nudgeTypeHelp(),
		Args:  cobra.ExactArgs(2),
		RunE:  runNudge,
	}
	killCmd := &cobra.Command{
		Use:   "kill <name>",
		Short: "Force-stop an agent",
		Args:  cobra.ExactArgs(1),
		RunE:  runKill,
	}
	killCmd.Flags().Bool("cleanup", false, "Also remove worktree")

	// --- Mail ---
	mailCmd := &cobra.Command{Use: "mail", Short: "Async mail system"}

	mailSendCmd := &cobra.Command{
		Use:   "send <to> <subject>",
		Short: "Send a message",
		Args:  cobra.ExactArgs(2),
		RunE:  runMailSend,
	}
	mailSendCmd.Flags().String("type", "status", "Message type")
	mailSendCmd.Flags().String("priority", "normal", "Priority: critical|normal|low")
	mailSendCmd.Flags().String("from", "human", "Sender")
	mailSendCmd.Flags().String("ref", "", "Related issue ID")
	mailSendCmd.Flags().StringP("body", "b", "", "Message body")

	mailReadCmd := &cobra.Command{
		Use:   "read [agent]",
		Short: "Read inbox",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runMailRead,
	}
	mailReadCmd.Flags().Bool("unread", false, "Only unread messages")

	mailLogCmd := &cobra.Command{
		Use:   "log",
		Short: "Message history",
		RunE:  runMailLog,
	}
	mailLogCmd.Flags().String("agent", "", "Filter by agent")
	mailLogCmd.Flags().String("type", "", "Filter by type")
	mailLogCmd.Flags().String("since", "", "Time filter (e.g. 1h, 30m)")

	mailCmd.AddCommand(mailSendCmd, mailReadCmd, mailLogCmd)

	// --- Memory ---
	memoryCmd := &cobra.Command{Use: "memory", Short: "Shared knowledge base"}

	memoryAddCmd := &cobra.Command{
		Use:   "add <type> <title>",
		Short: "Record a decision/discovery/convention",
		Args:  cobra.ExactArgs(2),
		RunE:  runMemoryAdd,
	}
	memoryAddCmd.Flags().String("context", "", "Context (decisions)")
	memoryAddCmd.Flags().String("rationale", "", "Rationale (decisions)")
	memoryAddCmd.Flags().String("decision", "", "Decision text")
	memoryAddCmd.Flags().String("finding", "", "Finding (discoveries)")
	memoryAddCmd.Flags().String("rule", "", "Rule (conventions)")
	memoryAddCmd.Flags().String("location", "", "Location (discoveries)")
	memoryAddCmd.Flags().String("affects", "", "Comma-separated affected issue IDs")
	memoryAddCmd.Flags().String("tags", "", "Comma-separated tags")
	memoryAddCmd.Flags().String("source", "", "Author (sets decided_by/discovered_by/established_by)")

	memorySearchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memory",
		Args:  cobra.ExactArgs(1),
		RunE:  runMemorySearch,
	}
	memorySearchCmd.Flags().Int("limit", 5, "Max results")

	memoryListCmd := &cobra.Command{
		Use:   "list",
		Short: "List memory entries",
		RunE:  runMemoryList,
	}
	memoryListCmd.Flags().String("type", "", "Filter by type: decision|discovery|convention")
	memoryListCmd.Flags().String("affects", "", "Filter by affected issue ID")

	memoryShowCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show memory entry detail",
		Args:  cobra.ExactArgs(1),
		RunE:  runMemoryShow,
	}

	memoryCmd.AddCommand(memoryAddCmd, memorySearchCmd, memoryListCmd, memoryShowCmd)

	// --- Worktree ---
	worktreeCmd := &cobra.Command{Use: "worktree", Short: "Git worktree management"}

	worktreeListCmd := &cobra.Command{
		Use:   "list",
		Short: "List worktrees",
		RunE:  runWorktreeList,
	}

	worktreeShowCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show worktree detail",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorktreeShow,
	}

	worktreeCleanupCmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove orphaned worktrees",
		RunE:  runWorktreeCleanup,
	}
	worktreeCleanupCmd.Flags().Bool("force", false, "Remove without prompting")

	worktreeCmd.AddCommand(worktreeListCmd, worktreeShowCmd, worktreeCleanupCmd)

	// --- Lock ---
	lockCmd := &cobra.Command{Use: "lock", Short: "File-level locks"}

	lockAcquireCmd := &cobra.Command{
		Use:   "acquire <file>",
		Short: "Acquire a lock",
		Args:  cobra.ExactArgs(1),
		RunE:  runLockAcquire,
	}
	lockAcquireCmd.Flags().String("agent", "human", "Agent name")
	lockAcquireCmd.Flags().String("issue", "", "Related issue ID")

	lockReleaseCmd := &cobra.Command{
		Use:   "release <file>",
		Short: "Release a lock",
		Args:  cobra.ExactArgs(1),
		RunE:  runLockRelease,
	}

	lockCheckCmd := &cobra.Command{
		Use:   "check <file>",
		Short: "Check lock status",
		Args:  cobra.ExactArgs(1),
		RunE:  runLockCheck,
	}

	lockCmd.AddCommand(lockAcquireCmd, lockReleaseCmd, lockCheckCmd)

	// --- Log ---
	logCmd := &cobra.Command{
		Use:   "log [agent]",
		Short: "View agent logs",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runLog,
	}
	logCmd.Flags().Bool("all", false, "Show all logs interleaved")

	logsDaemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Tail the daemon log",
		RunE:  runLogsDaemon,
	}
	logCmd.AddCommand(logsDaemonCmd)

	// --- Config ---
	configCmd := &cobra.Command{Use: "config", Short: "Configuration management"}
	configShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		RunE:  runConfigShow,
	}
	configSetCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}
	configCmd.AddCommand(configShowCmd, configSetCmd)

	// --- Utility ---
	spawnCmd := &cobra.Command{
		Use:   "spawn",
		Short: "Spawn a new agent",
		RunE:  runSpawn,
	}
	spawnCmd.Flags().String("role", "", "Agent role: lead|builder|reviewer|explorer|researcher")
	spawnCmd.Flags().String("issues", "", "Comma-separated issue IDs to assign")
	spawnCmd.Flags().String("spawned-by", "", "Parent agent ID (defaults to LOOM_AGENT_ID env var)")
	spawnCmd.Flags().String("slug", "", "Worktree slug for builders")
	spawnCmd.Flags().String("task", "", "Custom task message for the agent")
	spawnCmd.Flags().String("mode", "", "Kiro mode override: chat|acp")
	spawnCmd.Flags().String("model", "", "Model override: sonnet|opus|haiku (default: from config)")
	spawnCmd.MarkFlagRequired("role")

	gcCmd := &cobra.Command{
		Use:   "gc",
		Short: "Garbage collection",
		RunE:  runGC,
	}
	gcCmd.Flags().Bool("dry-run", false, "Show what would be cleaned")

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export work summary",
		RunE:  runExport,
	}
	exportCmd.Flags().String("issue", "", "Export summary for a specific issue")
	mcpServerCmd := &cobra.Command{
		Use:   "mcp-server",
		Short: "Start MCP server (stdio transport)",
		RunE:  runMCPServer,
	}
	mcpServerCmd.Flags().String("agent-id", "", "Agent ID for this MCP server instance (falls back to LOOM_AGENT_ID env var)")
	mcpServerCmd.Flags().String("loom-root", "", "Path to .loom directory (auto-detected if omitted)")

	mergeCmd := &cobra.Command{
		Use:   "merge <issue-id>",
		Short: "Squash-merge an issue's worktree branch into main",
		Args:  cobra.ExactArgs(1),
		RunE:  runMerge,
	}
	mergeCmd.Flags().StringP("message", "m", "", "Commit message (auto-generated if omitted)")
	mergeCmd.Flags().Bool("cleanup", false, "Remove worktree and branch after merge")

	mergesCmd := &cobra.Command{
		Use:   "merges",
		Short: "Show merge queue status",
		RunE:  runMerges,
	}

	findingCmd := &cobra.Command{
		Use:   "finding <message>",
		Short: "Send a finding to your lead agent",
		Args:  cobra.ExactArgs(1),
		RunE:  runFinding,
	}
	findingCmd.Flags().String("ref", "", "Related issue ID")

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update loom to the latest version",
		RunE:  runUpdate,
	}

	root.AddCommand(
		initCmd,
		startCmd, stopCmd, restartCmd, statusCmd,
		dashCmd, taskCmd,
		issueCmd,
		agentsCmd, agentCmd,
		attachCmd, nudgeCmd, killCmd,
		spawnCmd,
		mailCmd, memoryCmd, worktreeCmd, lockCmd,
		logCmd, configCmd,
		gcCmd, exportCmd, mcpServerCmd, mergeCmd, mergesCmd,
		findingCmd,
		updateCmd,
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func stub(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Run:   func(cmd *cobra.Command, args []string) { fmt.Println("not implemented yet") },
	}
}

func dashPidFile(root string) string {
	return filepath.Join(root, "dashboard.pid")
}

func isDashboardRunning(root string) bool {
	data, err := os.ReadFile(dashPidFile(root))
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func launchDashboard(root string) error {
	for {
		if isDashboardRunning(root) {
			fmt.Println("Dashboard is already running.")
			return nil
		}
		os.WriteFile(dashPidFile(root), []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
		m := dashboard.New(root)
		p := tea.NewProgram(m, dashboard.ProgramOptions()...)
		finalModel, err := p.Run()
		os.Remove(dashPidFile(root))
		if err != nil {
			return err
		}
		if dm, ok := finalModel.(dashboard.Model); ok && dm.Reloading() {
			// Binary changed — re-exec with the same arguments.
			self, execErr := os.Executable()
			if execErr != nil {
				return execErr
			}
			return syscall.Exec(self, os.Args, os.Environ())
		}
		return nil
	}
}

func runDash(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	return launchDashboard(root)
}

func runInit(cmd *cobra.Command, args []string) error {
	// Check git repo
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository (no .git/ found)")
	}

	force, _ := cmd.Flags().GetBool("force")
	refresh, _ := cmd.Flags().GetBool("refresh")

	if force && refresh {
		return fmt.Errorf("--force and --refresh are mutually exclusive")
	}

	// --refresh: update templates/agents/hooks only, preserve state
	if refresh {
		if _, err := os.Stat(".loom"); os.IsNotExist(err) {
			return fmt.Errorf(".loom/ does not exist (run loom init first)")
		}
		if err := installTemplates(); err != nil {
			return err
		}
		if err := installAgentsAndHooks(); err != nil {
			return err
		}
		fmt.Println("Refreshed templates, agents, and hooks")
		return nil
	}

	// Check existing .loom/
	if _, err := os.Stat(".loom"); err == nil {
		if !force {
			return fmt.Errorf(".loom/ already exists (use --force to overwrite)")
		}
		os.RemoveAll(".loom")
	}

	// Create directory structure
	dirs := []string{
		".loom/agents",
		".loom/issues",
		".loom/mail/inbox",
		".loom/mail/archive",
		".loom/memory/decisions",
		".loom/memory/discoveries",
		".loom/memory/conventions",
		".loom/worktrees",
		".loom/plans",
		".loom/artifacts/reviews",
		".loom/artifacts/research",
		".loom/artifacts/patches",
		".loom/locks",
		".loom/logs",
		".loom/templates",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	// Create counter files
	counters := []string{
		".loom/issues/counter.txt",
		".loom/memory/decisions/counter.txt",
		".loom/memory/discoveries/counter.txt",
		".loom/memory/conventions/counter.txt",
	}
	for _, c := range counters {
		if err := os.WriteFile(c, []byte("0"), 0644); err != nil {
			return fmt.Errorf("creating %s: %w", c, err)
		}
	}

	// Write default config
	cfg := config.DefaultConfig()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	cfg.Tmux.SessionName = config.DeriveSessionName(wd)
	cfg.Project = config.SanitizeBasename(wd)
	if err := config.Save(".loom", cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Copy embedded templates and install agents/hooks
	if err := installTemplates(); err != nil {
		return err
	}
	if err := installAgentsAndHooks(); err != nil {
		return err
	}

	// Update .gitignore
	if err := appendToGitignore(".loom/"); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}
	if err := appendToGitignore(".kiro/"); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}

	cliout.PrintSuccess("Initialized .loom/ in current directory")
	return nil
}

func appendToGitignore(entry string) error {
	path := ".gitignore"
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), entry) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Add newline before entry if file doesn't end with one
	if len(data) > 0 && data[len(data)-1] != '\n' {
		f.WriteString("\n")
	}
	_, err = f.WriteString(entry + "\n")
	return err
}

func installTemplates() error {
	if err := os.MkdirAll(".loom/templates", 0755); err != nil {
		return fmt.Errorf("creating .loom/templates: %w", err)
	}
	entries, err := fs.ReadDir(templates.TemplatesFS, ".")
	if err != nil {
		return fmt.Errorf("reading embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(templates.TemplatesFS, e.Name())
		if err != nil {
			return fmt.Errorf("reading template %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(".loom/templates", e.Name()), data, 0644); err != nil {
			return fmt.Errorf("writing template %s: %w", e.Name(), err)
		}
	}
	return nil
}

func installAgentsAndHooks() error {
	if err := os.MkdirAll(".kiro/agents", 0755); err != nil {
		return fmt.Errorf("creating .kiro/agents: %w", err)
	}
	if err := os.MkdirAll(".kiro/agents/prompts", 0755); err != nil {
		return fmt.Errorf("creating .kiro/agents/prompts: %w", err)
	}
	// Copy prompt templates
	tplEntries, err := fs.ReadDir(templates.TemplatesFS, ".")
	if err != nil {
		return fmt.Errorf("reading embedded templates: %w", err)
	}
	for _, e := range tplEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(templates.TemplatesFS, e.Name())
		if err != nil {
			continue
		}
		role := strings.TrimSuffix(e.Name(), ".md")
		if err := os.WriteFile(filepath.Join(".kiro/agents/prompts", "loom-"+role+".md"), data, 0644); err != nil {
			return fmt.Errorf("writing prompt %s: %w", e.Name(), err)
		}
	}
	// Copy agent definitions
	agentEntries, err := fs.ReadDir(agents.AgentsFS, ".")
	if err != nil {
		return fmt.Errorf("reading embedded agents: %w", err)
	}
	for _, e := range agentEntries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(agents.AgentsFS, e.Name())
		if err != nil {
			return fmt.Errorf("reading agent %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(".kiro/agents", e.Name()), data, 0644); err != nil {
			return fmt.Errorf("writing agent %s: %w", e.Name(), err)
		}
	}
	// Install hooks
	if err := os.MkdirAll(".kiro/hooks", 0755); err != nil {
		return fmt.Errorf("creating .kiro/hooks: %w", err)
	}
	hookEntries, err := fs.ReadDir(agents.AgentsFS, "hooks")
	if err != nil {
		return fmt.Errorf("reading embedded hooks: %w", err)
	}
	for _, e := range hookEntries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(agents.AgentsFS, "hooks/"+e.Name())
		if err != nil {
			return fmt.Errorf("reading hook %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(".kiro/hooks", e.Name()), data, 0755); err != nil {
			return fmt.Errorf("writing hook %s: %w", e.Name(), err)
		}
	}
	return nil
}

func runTask(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	iss, err := issue.Create(root, args[0], issue.CreateOpts{
		Type:     "task",
		Priority: "normal",
	})
	if err != nil {
		return err
	}
	cliout.PrintSuccess("Created "+args[0], iss.ID)
	cliout.PrintInfo("The orchestrator will pick this up automatically.")
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	return config.Set(root, args[0], args[1])
}

func runIssueCreate(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	typ, _ := cmd.Flags().GetString("type")
	priority, _ := cmd.Flags().GetString("priority")
	parent, _ := cmd.Flags().GetString("parent")
	desc, _ := cmd.Flags().GetString("description")
	depsStr, _ := cmd.Flags().GetString("depends-on")

	var deps []string
	if depsStr != "" {
		for _, d := range strings.Split(depsStr, ",") {
			if t := strings.TrimSpace(d); t != "" {
				deps = append(deps, t)
			}
		}
	}

	iss, err := issue.Create(root, args[0], issue.CreateOpts{
		Type:        typ,
		Priority:    priority,
		Parent:      parent,
		Description: desc,
		DependsOn:   deps,
	})
	if err != nil {
		return err
	}
	cliout.PrintSuccess("Created "+iss.Title, iss.ID)
	return nil
}

func runIssueList(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	status, _ := cmd.Flags().GetString("status")
	assignee, _ := cmd.Flags().GetString("assignee")
	typ, _ := cmd.Flags().GetString("type")
	all, _ := cmd.Flags().GetBool("all")
	tree, _ := cmd.Flags().GetBool("tree")

	issues, err := issue.List(root, issue.ListOpts{
		Status: status, Assignee: assignee, Type: typ, All: all,
	})
	if err != nil {
		return err
	}

	if tree {
		printTree(issues)
		return nil
	}

	cliout.PrintInfo(fmt.Sprintf("%-12s %-8s %-14s %-40s %s", "ID", "TYPE", "STATUS", "TITLE", "ASSIGNEE"))
	for _, iss := range issues {
		title := iss.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf("%-12s %-8s %-14s %-40s %s\n", iss.ID, iss.Type, iss.Status, title, iss.Assignee)
	}
	return nil
}

func printTree(issues []*issue.Issue) {
	byID := make(map[string]*issue.Issue)
	for _, iss := range issues {
		byID[iss.ID] = iss
	}

	// Find roots (no parent or parent not in set)
	var roots []*issue.Issue
	for _, iss := range issues {
		if iss.Parent == "" || byID[iss.Parent] == nil {
			roots = append(roots, iss)
		}
	}

	for _, r := range roots {
		printNode(r, byID, "")
	}
}

func printNode(iss *issue.Issue, byID map[string]*issue.Issue, prefix string) {
	assignee := ""
	if iss.Assignee != "" {
		assignee = " (" + iss.Assignee + ")"
	}
	fmt.Printf("%s%s [%s] [%s] %s%s\n", prefix, iss.ID, iss.Type, iss.Status, iss.Title, assignee)

	// Print dependency info
	for _, dep := range iss.DependsOn {
		connector := "    └── "
		if prefix != "" {
			connector = prefix + "    └── "
		}
		fmt.Printf("%sdepends on: %s\n", connector, dep)
	}

	// Print children
	for i, childID := range iss.Children {
		child, ok := byID[childID]
		if !ok {
			continue
		}
		isLast := i == len(iss.Children)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}
		printNodeChild(child, byID, prefix+connector, prefix+childPrefix)
	}
}

func printNodeChild(iss *issue.Issue, byID map[string]*issue.Issue, linePrefix, childPrefix string) {
	assignee := ""
	if iss.Assignee != "" {
		assignee = " (" + iss.Assignee + ")"
	}
	fmt.Printf("%s%s [%s] [%s] %s%s\n", linePrefix, iss.ID, iss.Type, iss.Status, iss.Title, assignee)

	for _, dep := range iss.DependsOn {
		fmt.Printf("%s└── depends on: %s\n", childPrefix, dep)
	}

	for i, childID := range iss.Children {
		child, ok := byID[childID]
		if !ok {
			continue
		}
		isLast := i == len(iss.Children)-1
		connector := "├── "
		nextPrefix := "│   "
		if isLast {
			connector = "└── "
			nextPrefix = "    "
		}
		printNodeChild(child, byID, childPrefix+connector, childPrefix+nextPrefix)
	}
}

func runIssueShow(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	iss, err := issue.Load(root, args[0])
	if err != nil {
		return err
	}

	cliout.PrintInfo(fmt.Sprintf("ID:          %s", iss.ID))
	fmt.Printf("Title:       %s\n", iss.Title)
	fmt.Printf("Type:        %s\n", iss.Type)
	fmt.Printf("Status:      %s\n", iss.Status)
	fmt.Printf("Priority:    %s\n", iss.Priority)
	if iss.Description != "" {
		fmt.Printf("Description: %s\n", iss.Description)
	}
	if iss.Assignee != "" {
		fmt.Printf("Assignee:    %s\n", iss.Assignee)
	}
	if iss.Parent != "" {
		fmt.Printf("Parent:      %s\n", iss.Parent)
	}
	if len(iss.DependsOn) > 0 {
		fmt.Printf("Depends On:  %s\n", strings.Join(iss.DependsOn, ", "))
	}
	if len(iss.Children) > 0 {
		fmt.Printf("Children:    %s\n", strings.Join(iss.Children, ", "))
	}
	if iss.Worktree != "" {
		fmt.Printf("Worktree:    %s\n", iss.Worktree)
	}
	fmt.Printf("Created By:  %s\n", iss.CreatedBy)
	fmt.Printf("Created At:  %s\n", iss.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated At:  %s\n", iss.UpdatedAt.Format("2006-01-02 15:04:05"))
	if iss.ClosedAt != nil {
		fmt.Printf("Closed At:   %s\n", iss.ClosedAt.Format("2006-01-02 15:04:05"))
	}
	if iss.CloseReason != "" {
		fmt.Printf("Close Reason: %s\n", iss.CloseReason)
	}

	if len(iss.History) > 0 {
		fmt.Println("\nHistory:")
		for _, h := range iss.History {
			detail := ""
			if h.Detail != "" {
				detail = " — " + h.Detail
			}
			fmt.Printf("  %s  %s  %s%s\n", h.At.Format("2006-01-02 15:04:05"), h.By, h.Action, detail)
		}
	}
	return nil
}

func runIssueUpdate(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	status, _ := cmd.Flags().GetString("status")
	priority, _ := cmd.Flags().GetString("priority")
	assignee, _ := cmd.Flags().GetString("assignee")

	if status == "" && priority == "" && assignee == "" {
		return fmt.Errorf("at least one of --status, --priority, or --assignee is required")
	}

	if status == "cancelled" {
		cancelled, err := issue.Cancel(root, args[0])
		if err != nil {
			return err
		}
		cliout.PrintWarning("Cancelled "+args[0])
		for _, ci := range cancelled {
			if ci.PreviousAssignee == "" {
				continue
			}
			msg := fmt.Sprintf("[LOOM] Issue %s cancelled. Stop work immediately.", ci.IssueID)
			if err := daemon.Message(root, ci.PreviousAssignee, msg); err != nil {
				cliout.PrintError("could not notify "+ci.PreviousAssignee, err.Error())
			}
		}
		return nil
	}

	_, err = issue.Update(root, args[0], issue.UpdateOpts{
		Status: status, Priority: priority, Assignee: assignee,
	})
	if err != nil {
		return err
	}
	cliout.PrintSuccess("Updated " + args[0])
	return nil
}

func runIssueClose(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	reason, _ := cmd.Flags().GetString("reason")
	_, err = issue.Close(root, args[0], reason)
	if err != nil {
		return err
	}
	cliout.PrintSuccess("Closed " + args[0])
	return nil
}

func runMailSend(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	typ, _ := cmd.Flags().GetString("type")
	priority, _ := cmd.Flags().GetString("priority")
	from, _ := cmd.Flags().GetString("from")
	if from == "human" {
		if envID := os.Getenv("LOOM_AGENT_ID"); envID != "" {
			from = envID
		}
	}
	ref, _ := cmd.Flags().GetString("ref")
	body, _ := cmd.Flags().GetString("body")

	if err := mail.Send(root, mail.SendOpts{
		From:     from,
		To:       args[0],
		Type:     typ,
		Priority: priority,
		Ref:      ref,
		Subject:  args[1],
		Body:     body,
	}); err != nil {
		return err
	}
	cliout.PrintSuccess("Sent to "+args[0]+": "+args[1])
	return nil
}

func runMailRead(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	agent := "orchestrator"
	if envID := os.Getenv("LOOM_AGENT_ID"); envID != "" {
		agent = envID
	}
	if len(args) > 0 {
		agent = args[0]
	}
	unreadOnly, _ := cmd.Flags().GetBool("unread")

	msgs, err := mail.Read(root, mail.ReadOpts{Agent: agent, UnreadOnly: unreadOnly})
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		fmt.Println("No messages")
		return nil
	}
	for _, m := range msgs {
		fmt.Printf("--- %s ---\n", m.ID)
		fmt.Printf("  Time:     %s\n", m.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("  From:     %s\n", m.From)
		fmt.Printf("  Type:     %s\n", m.Type)
		fmt.Printf("  Subject:  %s\n", m.Subject)
		if m.Body != "" {
			fmt.Printf("  Body:     %s\n", m.Body)
		}
		fmt.Println()
		if !m.Read {
			mail.MarkRead(root, agent, m.ID)
		}
	}
	return nil
}

func runMailLog(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	agent, _ := cmd.Flags().GetString("agent")
	typ, _ := cmd.Flags().GetString("type")
	sinceStr, _ := cmd.Flags().GetString("since")

	var since time.Duration
	if sinceStr != "" {
		since, err = time.ParseDuration(sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
	}

	msgs, err := mail.Log(root, mail.LogOpts{Agent: agent, Type: typ, Since: since})
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		fmt.Println("No messages")
		return nil
	}
	for _, m := range msgs {
		fmt.Printf("%s  %s → %s  [%s]  %s\n", m.Timestamp.Format("2006-01-02 15:04:05"), m.From, m.To, m.Type, m.Subject)
	}
	return nil
}

func runMemoryAdd(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	typ, title := args[0], args[1]
	ctx, _ := cmd.Flags().GetString("context")
	rationale, _ := cmd.Flags().GetString("rationale")
	decision, _ := cmd.Flags().GetString("decision")
	finding, _ := cmd.Flags().GetString("finding")
	rule, _ := cmd.Flags().GetString("rule")
	location, _ := cmd.Flags().GetString("location")
	affectsStr, _ := cmd.Flags().GetString("affects")
	tagsStr, _ := cmd.Flags().GetString("tags")
	source, _ := cmd.Flags().GetString("source")

	opts := memory.AddOpts{
		Type:      typ,
		Title:     title,
		Context:   ctx,
		Rationale: rationale,
		Decision:  decision,
		Finding:   finding,
		Rule:      rule,
		Location:  location,
		By:        source,
	}
	if affectsStr != "" {
		opts.Affects = splitCSV(affectsStr)
	}
	if tagsStr != "" {
		opts.Tags = splitCSV(tagsStr)
	}

	entry, err := memory.Add(root, opts)
	if err != nil {
		return err
	}
	cliout.PrintSuccess("Added "+entry.Title, entry.ID)
	return nil
}

func runMemorySearch(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
	results, err := memory.Search(root, memory.SearchOpts{Query: args[0], Limit: limit})
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("No results")
		return nil
	}
	fmt.Printf("Results (%d matches):\n\n", len(results))
	for i, r := range results {
		fmt.Printf("%d. [%s] %s\n", i+1, r.Entry.ID, r.Entry.Title)
		fmt.Printf("   Score: %.2f | Type: %s", r.Score, r.Entry.Type)
		if by := memory.ByField(r.Entry); by != "" {
			fmt.Printf(" | By: %s", by)
		}
		fmt.Println()
		if s := memory.Snippet(r.Entry); s != "" {
			fmt.Printf("   %s\n", s)
		}
		fmt.Println()
	}
	return nil
}

func runMemoryList(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	typ, _ := cmd.Flags().GetString("type")
	affects, _ := cmd.Flags().GetString("affects")

	entries, err := memory.List(root, memory.ListOpts{Type: typ, Affects: affects})
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No memory entries")
		return nil
	}
	fmt.Printf("%-12s %-12s %-40s %-16s %s\n", "ID", "TYPE", "TITLE", "BY", "TIMESTAMP")
	for _, e := range entries {
		title := e.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf("%-12s %-12s %-40s %-16s %s\n", e.ID, e.Type, title, memory.ByField(e), e.Timestamp.Format("2006-01-02 15:04"))
	}
	return nil
}

func runMemoryShow(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	e, err := memory.Load(root, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("ID:        %s\n", e.ID)
	fmt.Printf("Title:     %s\n", e.Title)
	fmt.Printf("Type:      %s\n", e.Type)
	fmt.Printf("Timestamp: %s\n", e.Timestamp.Format("2006-01-02 15:04:05"))
	printIf("Decided By", e.DecidedBy)
	printIf("Context", e.Context)
	printIf("Decision", e.Decision)
	printIf("Rationale", e.Rationale)
	for _, a := range e.Alternatives {
		fmt.Printf("Alternative: %s (rejected: %s)\n", a.Option, a.RejectedBecause)
	}
	printIf("Discovered By", e.DiscoveredBy)
	printIf("Location", e.Location)
	printIf("Finding", e.Finding)
	printIf("Implications", e.Implications)
	printIf("Established By", e.EstablishedBy)
	printIf("Rule", e.Rule)
	for _, ex := range e.Examples {
		fmt.Printf("Example:   %s\n", ex)
	}
	printIf("Applies To", e.AppliesTo)
	if len(e.Affects) > 0 {
		fmt.Printf("Affects:   %s\n", strings.Join(e.Affects, ", "))
	}
	if len(e.Tags) > 0 {
		fmt.Printf("Tags:      %s\n", strings.Join(e.Tags, ", "))
	}
	return nil
}

func printIf(label, value string) {
	if value != "" {
		fmt.Printf("%-10s %s\n", label+":", value)
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func runWorktreeList(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	wts, err := worktree.List(root)
	if err != nil {
		return err
	}
	if len(wts) == 0 {
		fmt.Println("No active worktrees")
		return nil
	}
	fmt.Printf("%-35s %-15s %-15s %s\n", "WORKTREE", "AGENT", "ISSUE", "BRANCH")
	for _, wt := range wts {
		fmt.Printf("%-35s %-15s %-15s %s\n", wt.Name, wt.Agent, wt.Issue, wt.Branch)
	}
	return nil
}

func runWorktreeShow(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	wt, stats, err := worktree.Show(root, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Name:    %s\n", wt.Name)
	fmt.Printf("Path:    %s\n", wt.Path)
	fmt.Printf("Branch:  %s\n", wt.Branch)
	fmt.Printf("Agent:   %s\n", wt.Agent)
	fmt.Printf("Issue:   %s\n", wt.Issue)
	if stats != nil && stats.FilesChanged > 0 {
		fmt.Printf("Changes: %d files changed (+%d, -%d)\n", stats.FilesChanged, stats.Insertions, stats.Deletions)
	}
	return nil
}

func runWorktreeCleanup(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	orphaned, err := worktree.Cleanup(root)
	if err != nil {
		return err
	}
	if len(orphaned) == 0 {
		fmt.Println("No orphaned worktrees")
		return nil
	}
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		fmt.Println("Orphaned worktrees:")
		for _, name := range orphaned {
			fmt.Printf("  %s\n", name)
		}
		fmt.Println("Use --force to remove them")
		return nil
	}
	for _, name := range orphaned {
		if err := worktree.Remove(root, name, true); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", name, err)
			continue
		}
		fmt.Printf("Removed %s\n", name)
	}
	return nil
}

func runLockAcquire(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	agent, _ := cmd.Flags().GetString("agent")
	issue, _ := cmd.Flags().GetString("issue")
	if err := lock.Acquire(root, lock.AcquireOpts{File: args[0], Agent: agent, Issue: issue}); err != nil {
		return err
	}
	fmt.Printf("Locked %s\n", args[0])
	return nil
}

func runLockRelease(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	if err := lock.Release(root, args[0]); err != nil {
		return err
	}
	fmt.Printf("Released %s\n", args[0])
	return nil
}

func runLockCheck(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	l, err := lock.Check(root, args[0])
	if err != nil {
		return err
	}
	if l == nil {
		fmt.Printf("%s is not locked\n", args[0])
		return nil
	}
	fmt.Printf("File:     %s\n", l.File)
	fmt.Printf("Agent:    %s\n", l.Agent)
	fmt.Printf("Issue:    %s\n", l.Issue)
	fmt.Printf("Acquired: %s\n", l.AcquiredAt.Format("2006-01-02 15:04:05"))
	return nil
}

func runAgents(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	agents, err := agent.List(root)
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		fmt.Println("No agents")
		return nil
	}
	fmt.Printf("%-16s %-12s %-10s %-25s %-15s %s\n", "ID", "ROLE", "STATUS", "WORKTREE", "ISSUES", "HEARTBEAT")
	for _, a := range agents {
		wt := "—"
		if a.WorktreeName != "" {
			wt = a.WorktreeName
		}
		issues := "—"
		if len(a.AssignedIssues) > 0 {
			issues = strings.Join(a.AssignedIssues, ",")
		}
		hb := relativeTime(a.Heartbeat)
		fmt.Printf("%-16s %-12s %-10s %-25s %-15s %s\n", a.ID, a.Role, a.Status, wt, issues, hb)
	}
	return nil
}

func runAgentShow(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	a, err := agent.Load(root, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("ID:          %s\n", a.ID)
	fmt.Printf("Role:        %s\n", a.Role)
	fmt.Printf("Status:      %s\n", a.Status)
	fmt.Printf("PID:         %d\n", a.PID)
	fmt.Printf("Tmux Target: %s\n", a.TmuxTarget)
	fmt.Printf("Spawned By:  %s\n", a.SpawnedBy)
	fmt.Printf("Spawned At:  %s\n", a.SpawnedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Heartbeat:   %s (%s)\n", a.Heartbeat.Format("2006-01-02 15:04:05"), relativeTime(a.Heartbeat))
	if len(a.AssignedIssues) > 0 {
		fmt.Printf("Issues:      %s\n", strings.Join(a.AssignedIssues, ", "))
	}
	if a.WorktreeName != "" {
		fmt.Printf("Worktree:    %s\n", a.WorktreeName)
	}
	fmt.Printf("Kiro Mode:   %s\n", a.Config.KiroMode)
	if a.Config.Model != "" {
		fmt.Printf("Model:       %s\n", a.Config.Model)
	}
	fmt.Printf("MCP Enabled: %v\n", a.Config.MCPEnabled)
	return nil
}

func runAgentHeartbeat(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	id := os.Getenv("LOOM_AGENT_ID")
	if id == "" {
		return fmt.Errorf("LOOM_AGENT_ID not set")
	}
	return agent.UpdateHeartbeat(root, id)
}

func runAgentCancel(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	if err := daemon.Cancel(root, args[0]); err != nil {
		return err
	}
	fmt.Printf("Cancelled session for %s\n", args[0])
	return nil
}

func runAttach(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	a, err := agent.Load(root, args[0])
	if err != nil {
		return err
	}
	return tmux.AttachSession(a.TmuxTarget)
}

func nudgeTypeHelp() string {
	var b strings.Builder
	for _, nt := range nudge.Types {
		fmt.Fprintf(&b, "  %-25s %s\n", nt.Key, nt.Label)
	}
	return b.String()
}

func runNudge(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	nt := nudge.ByKey(args[1])
	if nt == nil {
		return fmt.Errorf("unknown nudge type %q\n\nAvailable types:\n%s", args[1], nudgeTypeHelp())
	}
	a, err := agent.Load(root, args[0])
	if err != nil {
		return err
	}
	if a.Config.KiroMode == "acp" {
		if err := daemon.Nudge(root, args[0], nt.Message); err != nil {
			return err
		}
	} else {
		if err := tmux.RunInPane(a.TmuxTarget, nt.Message); err != nil {
			return err
		}
	}
	fmt.Printf("Nudged %s: %s\n", args[0], nt.Label)
	return nil
}

func runKill(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	cleanup, _ := cmd.Flags().GetBool("cleanup")
	a, err := agent.Load(root, args[0])
	if err != nil {
		return err
	}
	if a.Config.KiroMode == "acp" {
		if err := daemon.Kill(root, args[0], cleanup); err != nil {
			return err
		}
	} else {
		if err := agent.Kill(root, args[0], cleanup); err != nil {
			return err
		}
	}
	fmt.Printf("Killed %s\n", args[0])
	return nil
}

func runSpawn(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}

	role, _ := cmd.Flags().GetString("role")
	issuesStr, _ := cmd.Flags().GetString("issues")
	spawnedBy, _ := cmd.Flags().GetString("spawned-by")
	slug, _ := cmd.Flags().GetString("slug")
	task, _ := cmd.Flags().GetString("task")
	mode, _ := cmd.Flags().GetString("mode")
	model, _ := cmd.Flags().GetString("model")

	if spawnedBy == "" {
		spawnedBy = os.Getenv("LOOM_AGENT_ID")
	}

	var issues []string
	if issuesStr != "" {
		for _, s := range strings.Split(issuesStr, ",") {
			if t := strings.TrimSpace(s); t != "" {
				issues = append(issues, t)
			}
		}
	}

	var extra map[string]string
	if task != "" {
		extra = map[string]string{"task": task}
	}

	a, err := agent.Spawn(root, agent.SpawnOpts{
		Role:           role,
		SpawnedBy:      spawnedBy,
		AssignedIssues: issues,
		IssueSlug:      slug,
		ExtraContext:    extra,
		Mode:           mode,
		Model:          model,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Spawned %s (role: %s)\n", a.ID, a.Role)
	return nil
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func runExport(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	issueFilter, _ := cmd.Flags().GetString("issue")

	// Load closed/cancelled issues
	allIssues, err := issue.List(root, issue.ListOpts{All: true})
	if err != nil {
		return err
	}

	fmt.Println("# Loom Export")

	// Issues Completed
	fmt.Println("\n## Issues Completed")
	var printed bool
	for _, iss := range allIssues {
		if iss.Status != "done" && iss.Status != "cancelled" {
			continue
		}
		if issueFilter != "" && iss.ID != issueFilter && iss.Parent != issueFilter {
			continue
		}
		if iss.Parent == "" || (issueFilter != "" && iss.ID == issueFilter) {
			closedAt := ""
			if iss.ClosedAt != nil {
				closedAt = " (closed " + iss.ClosedAt.Format("2006-01-02") + ")"
			}
			fmt.Printf("- %s: %s [%s]%s\n", iss.ID, iss.Title, iss.Type, closedAt)
			// Print children
			for _, child := range allIssues {
				if child.Parent == iss.ID && (child.Status == "done" || child.Status == "cancelled") {
					childClosed := ""
					if child.ClosedAt != nil {
						childClosed = " (closed " + child.ClosedAt.Format("2006-01-02") + ")"
					}
					fmt.Printf("  - %s: %s [%s]%s\n", child.ID, child.Title, child.Type, childClosed)
				}
			}
			printed = true
		}
	}
	if !printed {
		fmt.Println("(none)")
	}

	// Memory entries
	memories, err := memory.List(root, memory.ListOpts{})
	if err != nil {
		memories = nil
	}

	// Decisions
	fmt.Println("\n## Decisions Made")
	printed = false
	for _, e := range memories {
		if e.Type != "decision" {
			continue
		}
		snippet := memory.Snippet(e)
		if snippet != "" {
			fmt.Printf("- %s: %s — %s\n", e.ID, e.Title, snippet)
		} else {
			fmt.Printf("- %s: %s\n", e.ID, e.Title)
		}
		printed = true
	}
	if !printed {
		fmt.Println("(none)")
	}

	// Discoveries
	fmt.Println("\n## Discoveries")
	printed = false
	for _, e := range memories {
		if e.Type != "discovery" {
			continue
		}
		fmt.Printf("- %s: %s\n", e.ID, e.Title)
		printed = true
	}
	if !printed {
		fmt.Println("(none)")
	}

	// Conventions
	fmt.Println("\n## Conventions Established")
	printed = false
	for _, e := range memories {
		if e.Type != "convention" {
			continue
		}
		fmt.Printf("- %s: %s\n", e.ID, e.Title)
		printed = true
	}
	if !printed {
		fmt.Println("(none)")
	}

	return nil
}

func runGC(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var total int

	// Archived mail older than 7 days
	archiveDir := filepath.Join(root, "mail", "archive")
	total += cleanOldFiles(archiveDir, cutoff, dryRun, "archived mail")

	// Logs older than 7 days
	logsDir := filepath.Join(root, "logs")
	total += cleanOldFiles(logsDir, cutoff, dryRun, "log files")

	// Dead agent registrations
	agents, _ := agent.List(root)
	liveAgents := make(map[string]bool)
	deadCount := 0
	for _, a := range agents {
		if a.Status == "dead" {
			if dryRun {
				fmt.Printf("[dry-run] Would kill and remove dead agent: %s (pid %d)\n", a.ID, a.PID)
			} else {
				if agent.KillProcess(a) {
					fmt.Printf("Killed orphaned process: %s (pid %d)\n", a.ID, a.PID)
				}
				if err := agent.Deregister(root, a.ID); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to remove agent %s: %v\n", a.ID, err)
					continue
				}
				fmt.Printf("Removed dead agent: %s\n", a.ID)
			}
			deadCount++
		} else {
			liveAgents[a.ID] = true
		}
	}
	total += deadCount

	// Orphaned kiro-cli ACP processes not tracked by any registered agent
	knownPIDs := make(map[int]bool)
	for _, a := range agents {
		if a.PID > 0 {
			knownPIDs[a.PID] = true
		}
	}
	if out, err := exec.Command("pgrep", "-f", "kiro-cli acp").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil || pid <= 0 || knownPIDs[pid] {
				continue
			}
			if dryRun {
				fmt.Printf("[dry-run] Would kill orphaned kiro-cli ACP process: %d\n", pid)
			} else {
				syscall.Kill(-pid, syscall.SIGTERM)
				time.Sleep(100 * time.Millisecond)
				syscall.Kill(-pid, syscall.SIGKILL)
				fmt.Printf("Killed orphaned kiro-cli ACP process: %d\n", pid)
			}
			total++
		}
	}

	// Stale mail inboxes for non-existent agents
	inboxDir := filepath.Join(root, "mail", "inbox")
	if entries, err := os.ReadDir(inboxDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && !liveAgents[e.Name()] {
				dir := filepath.Join(inboxDir, e.Name())
				if dryRun {
					fmt.Printf("[dry-run] Would remove stale inbox: %s\n", e.Name())
				} else {
					os.RemoveAll(dir)
					fmt.Printf("Removed stale inbox: %s\n", e.Name())
				}
				total++
			}
		}
	}

	// Orphan worktrees (not associated with any registered agent)
	if orphans, err := worktree.Cleanup(root); err == nil {
		for _, name := range orphans {
			if dryRun {
				fmt.Printf("[dry-run] Would remove orphan worktree: %s\n", name)
			} else {
				if err := worktree.Remove(root, name, true); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to remove worktree %s: %v\n", name, err)
					continue
				}
				fmt.Printf("Removed orphan worktree: %s\n", name)
			}
			total++
		}
	}

	// Stale locks held by non-existent agents
	if locks, err := lock.ListLocks(root); err == nil {
		for _, l := range locks {
			if !liveAgents[l.Agent] {
				if dryRun {
					fmt.Printf("[dry-run] Would release stale lock: %s (agent %s)\n", l.File, l.Agent)
				} else {
					if err := lock.Release(root, l.File); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to release lock %s: %v\n", l.File, err)
						continue
					}
					fmt.Printf("Released stale lock: %s (agent %s)\n", l.File, l.Agent)
				}
				total++
			}
		}
	}

	if dryRun {
		fmt.Printf("\nDry run: %d items would be cleaned\n", total)
	} else {
		fmt.Printf("\nCleaned %d items\n", total)
	}
	return nil
}

func cleanOldFiles(dir string, cutoff time.Time, dryRun bool, label string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if dryRun {
				fmt.Printf("[dry-run] Would remove %s: %s\n", label, e.Name())
			} else {
				os.Remove(filepath.Join(dir, e.Name()))
				fmt.Printf("Removed %s: %s\n", label, e.Name())
			}
			count++
		}
	}
	return count
}

func runLogsDaemon(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	logPath := filepath.Join(root, "logs", "daemon.log")
	tail := exec.Command("tail", "-f", logPath)
	tail.Stdout = os.Stdout
	tail.Stderr = os.Stderr
	return tail.Run()
}

func runLog(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	all, _ := cmd.Flags().GetBool("all")

	logsDir := filepath.Join(root, "logs")

	if all {
		entries, err := os.ReadDir(logsDir)
		if err != nil {
			fmt.Println("No logs found")
			return nil
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(logsDir, e.Name()))
			if err != nil {
				continue
			}
			fmt.Print(string(data))
		}
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("specify an agent name or use --all")
	}

	logFile := filepath.Join(logsDir, args[0]+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		fmt.Printf("No logs for %s\n", args[0])
		return nil
	}
	fmt.Print(string(data))
	return nil
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	agentID, _ := cmd.Flags().GetString("agent-id")
	if agentID == "" {
		agentID = os.Getenv("LOOM_AGENT_ID")
	}
	root, _ := cmd.Flags().GetString("loom-root")
	if root == "" {
		root = os.Getenv("LOOM_ROOT")
	}
	if root == "" {
		var err error
		root, err = config.FindLoomRoot()
		if err != nil {
			return err
		}
	}
	srv := &mcp.Server{LoomRoot: root, AgentID: agentID}
	return srv.Run()
}

func runStart(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}

	// Check prerequisites
	for _, bin := range []string{"tmux", "kiro-cli", "loom"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found in PATH (required)", bin)
		}
	}

	pid, alive := daemon.CheckLock(root)
	if alive {
		return fmt.Errorf("loom already running (pid %d)", pid)
	}

	// Daemonize via re-exec with sentinel env var
	if os.Getenv("LOOM_DAEMON") != "1" {
		logPath := filepath.Join(root, "logs", "daemon.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening daemon log: %w", err)
		}

		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding executable: %w", err)
		}

		child := exec.Command(self, os.Args[1:]...)
		child.Env = append(os.Environ(), "LOOM_DAEMON=1")
		child.Stdout = logFile
		child.Stderr = logFile
		if err := child.Start(); err != nil {
			logFile.Close()
			return fmt.Errorf("daemonizing: %w", err)
		}
		logFile.Close()

		cfg, _ := config.Load(root)
		session := "loom"
		if cfg != nil {
			session = cfg.Tmux.SessionName
		}
		fmt.Println("Loom started in background.")
		fmt.Printf("  Logs:   loom logs daemon  (or tail %s)\n", logPath)
		fmt.Printf("  Status: loom status\n")
		fmt.Printf("  Dash:   loom dash\n")
		fmt.Printf("  Stop:   loom stop\n")
		fmt.Printf("  Tmux:   tmux attach -t %s\n", session)

		noDash, _ := cmd.Flags().GetBool("no-dashboard")
		if !noDash {
			return launchDashboard(root)
		}
		return nil
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	// Create tmux session (resolve name collisions with other projects)
	desired := cfg.Tmux.SessionName
	if tmux.SessionExists(desired) {
		// If we own this session (lock is ours), reuse it.
		// Otherwise, find a unique suffix.
		_, alive := daemon.CheckLock(root)
		if !alive {
			for i := 2; i <= 99; i++ {
				candidate := fmt.Sprintf("%s-%d", desired, i)
				if !tmux.SessionExists(candidate) {
					fmt.Fprintf(os.Stderr, "Warning: tmux session %q already exists, using %q\n", desired, candidate)
					cfg.Tmux.SessionName = candidate
					break
				}
			}
			if cfg.Tmux.SessionName == desired {
				return fmt.Errorf("tmux session %q (and suffixed variants) already exist", desired)
			}
		}
	}
	if !tmux.SessionExists(cfg.Tmux.SessionName) {
		if err := tmux.CreateSession(cfg.Tmux.SessionName); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}
	}

	if err := daemon.AcquireLock(root); err != nil {
		return err
	}

	resume, _ := cmd.Flags().GetBool("resume")
	mode, _ := cmd.Flags().GetString("mode")
	if resume {
		// Re-queue active ACP agents so watchPendingAgents re-activates them.
		// Their kiro-cli processes died when the old daemon exited.
		agents, _ := agent.List(root)
		for _, a := range agents {
			if a.Config.KiroMode == "acp" && (a.Status == "active" || a.Status == "activating") {
				agent.KillProcess(a) // clean up orphaned kiro-cli
				a.Status = "pending-acp"
				agent.Save(root, a)
			}
		}
	} else {
		_, err = agent.Spawn(root, agent.SpawnOpts{
			Role:         "orchestrator",
			ExtraContext: map[string]string{"task": "You are now online. Check for open issues with loom issue list and process any that are unassigned. Then wait for new issue notifications."},
			Mode:         mode,
		})
		if err != nil {
			daemon.ReleaseLock(root)
			return fmt.Errorf("spawning orchestrator: %w", err)
		}
	}

	// Kill any orphaned daemon processes from previous runs to prevent
	// dual-daemon races where two daemons mark each other's agents dead.
	daemon.KillStaleDaemons()

	// Start daemon goroutines
	d := daemon.New(root, cfg)
	if err := d.Start(); err != nil {
		daemon.ReleaseLock(root)
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Block on signals: SIGHUP triggers hot-reload, SIGINT/SIGTERM shut down.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for s := range sig {
		if s == syscall.SIGHUP {
			log.Println("[daemon] received SIGHUP, reloading")
			if err := d.Reload(); err != nil {
				log.Printf("[daemon] reload failed: %v", err)
			}
			continue
		}
		break
	}

	d.Stop()
	daemon.ReleaseLock(root)
	return nil
}

func runRestart(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}

	pid, alive := daemon.CheckLock(root)
	if !alive {
		return fmt.Errorf("loom is not running")
	}

	// Send SIGHUP to trigger in-process reload (preserves ACP clients).
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process %d: %w", pid, err)
	}
	if err := p.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("signaling daemon: %w", err)
	}
	fmt.Printf("Sent SIGHUP to daemon (pid %d) — reloading\n", pid)

	noDash, _ := cmd.Flags().GetBool("no-dashboard")
	if !noDash {
		return launchDashboard(root)
	}
	return nil
}

func runUpdate(cmd *cobra.Command, args []string) error {
	fmt.Printf("Current: %s (%s)\n", version, commitHash)

	const repoAPI = "https://api.github.com/repos/kv0409/loom/releases/latest"

	fmt.Print("Checking for updates... ")
	req, _ := http.NewRequest("GET", repoAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("checking releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("parsing release: %w", err)
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	fmt.Printf("latest is %s\n", latest)

	if latest == version {
		fmt.Println("Already up to date.")
		return nil
	}

	// Find the asset for this OS/arch
	want := fmt.Sprintf("loom_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	var assetURL string
	for _, a := range release.Assets {
		if a.Name == want {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		return fmt.Errorf("no release binary for %s/%s (looked for %s)", runtime.GOOS, runtime.GOARCH, want)
	}

	// Download to temp file
	fmt.Printf("Downloading %s... ", want)
	dlResp, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	defer dlResp.Body.Close()

	tmpFile, err := os.CreateTemp("", "loom-update-*.tar.gz")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("saving download: %w", err)
	}
	tmpFile.Close()
	fmt.Println("done.")

	// Extract binary from tarball
	fmt.Print("Installing... ")
	extractDir, err := os.MkdirTemp("", "loom-extract-*")
	if err != nil {
		return fmt.Errorf("creating extract dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	tar := exec.Command("tar", "xzf", tmpPath, "-C", extractDir)
	if out, err := tar.CombinedOutput(); err != nil {
		return fmt.Errorf("extracting: %s\n%s", err, out)
	}

	newBin := filepath.Join(extractDir, "loom")
	if _, err := os.Stat(newBin); err != nil {
		return fmt.Errorf("extracted binary not found: %w", err)
	}

	// Replace current binary
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding current executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	// Atomic replace: copy new binary to temp next to target, then rename
	destDir := filepath.Dir(self)
	staged, err := os.CreateTemp(destDir, ".loom-update-*")
	if err != nil {
		return fmt.Errorf("staging new binary: %w", err)
	}
	stagedPath := staged.Name()

	src, err := os.Open(newBin)
	if err != nil {
		staged.Close()
		os.Remove(stagedPath)
		return fmt.Errorf("opening new binary: %w", err)
	}
	if _, err := io.Copy(staged, src); err != nil {
		src.Close()
		staged.Close()
		os.Remove(stagedPath)
		return fmt.Errorf("copying new binary: %w", err)
	}
	src.Close()
	staged.Close()

	if err := os.Chmod(stagedPath, 0755); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(stagedPath, self); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("replacing binary: %w", err)
	}
	fmt.Printf("done. Updated to %s\n", latest)

	// Restart daemon if running
	root, err := config.FindLoomRoot()
	if err != nil {
		return nil
	}
	pid, alive := daemon.CheckLock(root)
	if !alive {
		return nil
	}

	fmt.Print("Restarting daemon... ")
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding daemon process: %w", err)
	}
	p.Signal(syscall.SIGTERM)
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, alive := daemon.CheckLock(root); !alive {
			break
		}
	}

	logPath := filepath.Join(root, "logs", "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening daemon log: %w", err)
	}
	child := exec.Command(self, "start", "--resume")
	child.Env = append(os.Environ(), "LOOM_DAEMON=1")
	child.Stdout = logFile
	child.Stderr = logFile
	if err := child.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("restarting daemon: %w", err)
	}
	logFile.Close()
	fmt.Printf("done (pid %d).\n", child.Process.Pid)
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}

	pid, alive := daemon.CheckLock(root)
	if !alive {
		return fmt.Errorf("loom is not running")
	}

	daemonOnly, _ := cmd.Flags().GetBool("daemon-only")
	clean, _ := cmd.Flags().GetBool("clean")
	if !daemonOnly {
		// Kill all registered agents
		agents, _ := agent.List(root)
		cfg, _ := config.Load(root)
		for _, a := range agents {
			fmt.Printf("Killing agent %s...\n", a.ID)
			agent.Kill(root, a.ID, clean)
		}

		// Remove worktrees — only force-remove if --clean
		if clean {
			wts, _ := worktree.List(root)
			for _, wt := range wts {
				fmt.Printf("Removing worktree %s...\n", wt.Name)
				worktree.Remove(root, wt.Name, true)
			}
		}

		// Kill the tmux session
		if cfg != nil {
			tmux.KillSession(cfg.Tmux.SessionName)
		}
	}

	force, _ := cmd.Flags().GetBool("force")
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process %d: %w", pid, err)
	}

	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := p.Signal(sig); err != nil {
		// Process may already be dead from tmux kill
		daemon.ReleaseLock(root)
	}

	// Kill any orphaned daemon processes that survived previous stops.
	daemon.KillStaleDaemons()

	fmt.Printf("Loom stopped (pid %d)\n", pid)
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}

	pid, alive := daemon.CheckLock(root)
	if !alive {
		fmt.Println("Loom is not running")
		return nil
	}

	fmt.Printf("Loom: running (pid %d)\n", pid)

	// Agent counts
	agents, _ := agent.List(root)
	var active, idle, dead int
	for _, a := range agents {
		switch a.Status {
		case "active":
			active++
		case "idle":
			idle++
		case "dead":
			dead++
		}
	}
	fmt.Printf("Agents: %d active, %d idle, %d dead\n", active, idle, dead)

	// Issue counts
	allIssues, _ := issue.List(root, issue.ListOpts{All: true})
	var open, inProgress, done int
	for _, iss := range allIssues {
		switch iss.Status {
		case "open", "assigned":
			open++
		case "in-progress", "review", "blocked":
			inProgress++
		case "done":
			done++
		}
	}
	fmt.Printf("Issues: %d open, %d in-progress, %d done\n", open, inProgress, done)

	// Worktree count
	wts, _ := worktree.List(root)
	fmt.Printf("Worktrees: %d active\n", len(wts))

	// Undelivered mail count
	var undelivered int
	inboxRoot := filepath.Join(root, "mail", "inbox")
	if entries, err := os.ReadDir(inboxRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			msgs, err := mail.Read(root, mail.ReadOpts{Agent: e.Name(), UnreadOnly: true})
			if err == nil {
				undelivered += len(msgs)
			}
		}
	}
	fmt.Printf("Mail: %d undelivered\n", undelivered)
	return nil
}

func runMerge(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	projectRoot := filepath.Dir(root)
	issueID := args[0]

	// Find the worktree/branch for this issue.
	wts, err := worktree.List(root)
	if err != nil {
		return err
	}
	var wt *worktree.Worktree
	for _, w := range wts {
		if w.Issue == issueID {
			wt = w
			break
		}
	}
	if wt == nil {
		// Also check git branches directly (worktree may have been removed but branch kept).
		out, err := exec.Command("git", "-C", projectRoot, "branch", "--list", issueID+"-*").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				branch := strings.TrimSpace(strings.TrimPrefix(line, "* "))
				if branch != "" && strings.HasPrefix(branch, issueID) {
					wt = &worktree.Worktree{Name: branch, Branch: branch, Issue: issueID}
					break
				}
			}
		}
	}
	if wt == nil {
		return fmt.Errorf("no worktree or branch found for issue %s", issueID)
	}

	// Merge.
	mergeCmd := exec.Command("git", "merge", "--squash", wt.Branch)
	mergeCmd.Dir = projectRoot
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge --squash %s: %s", wt.Branch, strings.TrimSpace(string(out)))
	}

	// Commit.
	msg, _ := cmd.Flags().GetString("message")
	if msg == "" {
		msg = fmt.Sprintf("Merge %s (%s)", wt.Branch, issueID)
	}
	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = projectRoot
	commitOut, err := commitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit: %s", strings.TrimSpace(string(commitOut)))
	}

	// Get the commit hash.
	hashCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	hashCmd.Dir = projectRoot
	hashOut, _ := hashCmd.Output()
	hash := strings.TrimSpace(string(hashOut))

	// Set merged_at on the issue.
	iss, err := issue.Load(root, issueID)
	if err == nil {
		now := time.Now()
		iss.MergedAt = &now
		issue.Save(root, iss)
	}

	fmt.Printf("Merged %s → %s\n", wt.Branch, hash)

	// Optionally clean up all worktrees for this issue.
	cleanup, _ := cmd.Flags().GetBool("cleanup")
	if cleanup {
		allForIssue, _ := worktree.ListForIssue(root, issueID)
		for _, w := range allForIssue {
			if err := worktree.Remove(root, w.Name, true); err != nil {
				if err2 := worktree.ForceRemove(root, w.Name); err2 != nil {
					fmt.Fprintf(os.Stderr, "Warning: cleanup failed for %s: %v\n", w.Name, err2)
				} else {
					fmt.Printf("Force-removed stale worktree %s\n", w.Name)
				}
			} else {
				fmt.Printf("Removed worktree %s\n", w.Name)
			}
		}
	}

	return nil
}

func runMerges(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	wts, err := worktree.List(root)
	if err != nil {
		return err
	}
	if len(wts) == 0 {
		fmt.Println("No worktrees in merge queue")
		return nil
	}

	issues, _ := issue.List(root, issue.ListOpts{All: true})
	issueByWT := map[string]*issue.Issue{}
	for _, iss := range issues {
		if iss.Worktree != "" {
			issueByWT[iss.Worktree] = iss
		}
	}

	fmt.Printf("%-30s %-30s %-12s %s\n", "WORKTREE", "BRANCH", "ISSUE", "STAGE")
	fmt.Println(strings.Repeat("─", 80))
	for _, wt := range wts {
		issueID := "—"
		stage := "ready"
		if iss, ok := issueByWT[wt.Name]; ok {
			issueID = iss.ID
			switch iss.Status {
			case "in-progress":
				stage = "building"
			case "review":
				stage = "review"
			case "done":
				stage = "merged"
			}
		}
		fmt.Printf("%-30s %-30s %-12s %s\n", truncStr(wt.Name, 30), truncStr(wt.Branch, 30), issueID, stage)
	}
	return nil
}

func truncStr(s string, n int) string {
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func runFinding(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}

	from := os.Getenv("LOOM_AGENT_ID")
	if from == "" {
		from = "human"
	}

	// Resolve lead: prefer LOOM_PARENT_AGENT env, fall back to SpawnedBy in registry.
	to := os.Getenv("LOOM_PARENT_AGENT")
	if to == "" && from != "human" {
		a, err := agent.Load(root, from)
		if err == nil && a.SpawnedBy != "" {
			to = a.SpawnedBy
		}
	}
	if to == "" {
		return fmt.Errorf("could not determine lead: set LOOM_PARENT_AGENT or ensure agent is registered with a spawned_by value")
	}

	ref, _ := cmd.Flags().GetString("ref")
	if err := mail.Send(root, mail.SendOpts{
		From:    from,
		To:      to,
		Type:    "finding",
		Subject: args[0],
		Ref:     ref,
	}); err != nil {
		return err
	}
	fmt.Printf("Finding sent to %s\n", to)
	return nil
}
