package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/templates"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func main() {
	root := &cobra.Command{
		Use:   "loom",
		Short: "Multi-agent orchestration for kiro-cli",
	}

	// --- loom init ---
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .loom/ in the current git repository",
		RunE:  runInit,
	}
	initCmd.Flags().Bool("force", false, "Overwrite existing .loom/ directory")

	// --- Lifecycle ---
	lifecycleGroup := &cobra.Group{ID: "lifecycle", Title: "Lifecycle"}
	root.AddGroup(lifecycleGroup)

	startCmd := stub("start", "Launch orchestrator and daemon")
	startCmd.GroupID = "lifecycle"
	stopCmd := stub("stop", "Graceful shutdown")
	stopCmd.GroupID = "lifecycle"
	statusCmd := stub("status", "Quick health check")
	statusCmd.GroupID = "lifecycle"

	// --- Dashboard ---
	dashCmd := stub("dash", "Launch TUI dashboard")

	// --- Task ---
	taskCmd := stub("task", "Create a task from natural language")

	// --- Issues ---
	issueCmd := &cobra.Command{Use: "issue", Short: "Issue tracker"}
	issueCmd.AddCommand(
		stub("create", "Create a new issue"),
		stub("list", "List issues"),
		stub("show", "Show issue detail"),
		stub("update", "Update an issue"),
		stub("close", "Close an issue"),
	)

	// --- Agents ---
	agentsCmd := stub("agents", "List all agents")
	agentCmd := &cobra.Command{Use: "agent", Short: "Agent management"}
	agentCmd.AddCommand(stub("show", "Show agent detail"))

	attachCmd := stub("attach", "Attach to agent tmux pane")
	attachCmd.Use = "attach <name>"
	nudgeCmd := stub("nudge", "Send message to agent")
	nudgeCmd.Use = "nudge <name> <message>"
	killCmd := stub("kill", "Force-stop an agent")
	killCmd.Use = "kill <name>"

	// --- Mail ---
	mailCmd := &cobra.Command{Use: "mail", Short: "Async mail system"}
	mailCmd.AddCommand(
		stub("send", "Send a message"),
		stub("read", "Read inbox"),
		stub("log", "Message history"),
	)

	// --- Memory ---
	memoryCmd := &cobra.Command{Use: "memory", Short: "Shared knowledge base"}
	memoryCmd.AddCommand(
		stub("add", "Record a decision/discovery/convention"),
		stub("search", "Search memory"),
		stub("list", "List memory entries"),
		stub("show", "Show memory entry detail"),
	)

	// --- Worktree ---
	worktreeCmd := &cobra.Command{Use: "worktree", Short: "Git worktree management"}
	worktreeCmd.AddCommand(
		stub("list", "List worktrees"),
		stub("show", "Show worktree detail"),
		stub("cleanup", "Remove orphaned worktrees"),
	)

	// --- Lock ---
	lockCmd := &cobra.Command{Use: "lock", Short: "File-level locks"}
	lockCmd.AddCommand(
		stub("acquire", "Acquire a lock"),
		stub("release", "Release a lock"),
		stub("check", "Check lock status"),
	)

	// --- Log ---
	logCmd := stub("log", "View agent logs")

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
	gcCmd := stub("gc", "Garbage collection")
	exportCmd := stub("export", "Export work summary")
	mcpServerCmd := stub("mcp-server", "Start MCP server")

	root.AddCommand(
		initCmd,
		startCmd, stopCmd, statusCmd,
		dashCmd, taskCmd,
		issueCmd,
		agentsCmd, agentCmd,
		attachCmd, nudgeCmd, killCmd,
		mailCmd, memoryCmd, worktreeCmd, lockCmd,
		logCmd, configCmd,
		gcCmd, exportCmd, mcpServerCmd,
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

func runInit(cmd *cobra.Command, args []string) error {
	// Check git repo
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository (no .git/ found)")
	}

	force, _ := cmd.Flags().GetBool("force")

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
	if err := config.Save(".loom", cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Copy embedded templates
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

	// Update .gitignore
	if err := appendToGitignore(".loom/"); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}

	fmt.Println("Initialized .loom/ in current directory")
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
