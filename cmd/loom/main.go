package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/karanagi/loom/internal/agent"
	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/daemon"
	"github.com/karanagi/loom/internal/dashboard"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/lock"
	"github.com/karanagi/loom/internal/mail"
	"github.com/karanagi/loom/internal/mcp"
	"github.com/karanagi/loom/internal/memory"
	"github.com/karanagi/loom/internal/tmux"
	"github.com/karanagi/loom/internal/worktree"
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

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Launch orchestrator and daemon",
		RunE:  runStart,
	}
	startCmd.Flags().Bool("resume", false, "Auto-resume without prompting")
	startCmd.Flags().Bool("fresh", false, "Discard previous state")
	startCmd.GroupID = "lifecycle"

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Graceful shutdown",
		RunE:  runStop,
	}
	stopCmd.Flags().Bool("force", false, "Send SIGKILL instead of SIGTERM")
	stopCmd.GroupID = "lifecycle"

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
	agentCmd.AddCommand(agentShowCmd, agentHeartbeatCmd)

	attachCmd := &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach to agent tmux pane",
		Args:  cobra.ExactArgs(1),
		RunE:  runAttach,
	}
	nudgeCmd := &cobra.Command{
		Use:   "nudge <name> <message>",
		Short: "Send message to agent",
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
	mcpServerCmd := &cobra.Command{
		Use:   "mcp-server",
		Short: "Start MCP server (stdio transport)",
		RunE:  runMCPServer,
	}
	mcpServerCmd.Flags().String("agent-id", "", "Agent ID for this MCP server instance (required)")
	mcpServerCmd.Flags().String("loom-root", "", "Path to .loom directory (auto-detected if omitted)")
	mcpServerCmd.MarkFlagRequired("agent-id")

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

func runDash(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	m := dashboard.New(root)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
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
	fmt.Printf("Created %s: %s\n", iss.ID, args[0])
	fmt.Println("The orchestrator will pick this up automatically.")
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
	fmt.Printf("Created %s: %s\n", iss.ID, iss.Title)
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

	fmt.Printf("%-12s %-8s %-14s %-40s %s\n", "ID", "TYPE", "STATUS", "TITLE", "ASSIGNEE")
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

	fmt.Printf("ID:          %s\n", iss.ID)
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

	_, err = issue.Update(root, args[0], issue.UpdateOpts{
		Status: status, Priority: priority, Assignee: assignee,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Updated %s\n", args[0])
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
	fmt.Printf("Closed %s\n", args[0])
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
	ref, _ := cmd.Flags().GetString("ref")
	body, _ := cmd.Flags().GetString("body")

	msg := &mail.Message{
		From:     from,
		To:       args[0],
		Type:     typ,
		Priority: priority,
		Ref:      ref,
		Subject:  args[1],
		Body:     body,
	}
	if err := mail.Send(root, msg); err != nil {
		return err
	}
	fmt.Printf("Sent to %s: %s\n", args[0], args[1])
	return nil
}

func runMailRead(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	agent := "orchestrator"
	if len(args) > 0 {
		agent = args[0]
	}
	unreadOnly, _ := cmd.Flags().GetBool("unread")

	msgs, err := mail.Read(root, agent, unreadOnly)
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

	entry := &memory.Entry{
		Type:      typ,
		Title:     title,
		Context:   ctx,
		Rationale: rationale,
		Decision:  decision,
		Finding:   finding,
		Rule:      rule,
		Location:  location,
	}
	if affectsStr != "" {
		entry.Affects = splitCSV(affectsStr)
	}
	if tagsStr != "" {
		entry.Tags = splitCSV(tagsStr)
	}
	switch typ {
	case "decision":
		entry.DecidedBy = source
	case "discovery":
		entry.DiscoveredBy = source
	case "convention":
		entry.EstablishedBy = source
	}

	if err := memory.Add(root, entry); err != nil {
		return err
	}
	fmt.Printf("Added %s: %s\n", entry.ID, entry.Title)
	return nil
}

func runMemorySearch(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
	results, err := memory.Search(root, args[0], limit)
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
		if err := worktree.Remove(root, name); err != nil {
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
	if err := lock.Acquire(root, args[0], agent, issue); err != nil {
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

func runNudge(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	a, err := agent.Load(root, args[0])
	if err != nil {
		return err
	}
	msg := "[LOOM] Nudge: " + args[1]
	if err := tmux.RunInPane(a.TmuxTarget, msg); err != nil {
		return err
	}
	fmt.Printf("Nudged %s\n", args[0])
	return nil
}

func runKill(cmd *cobra.Command, args []string) error {
	root, err := config.FindLoomRoot()
	if err != nil {
		return err
	}
	cleanup, _ := cmd.Flags().GetBool("cleanup")
	if err := agent.Kill(root, args[0], cleanup); err != nil {
		return err
	}
	fmt.Printf("Killed %s\n", args[0])
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

func runMCPServer(cmd *cobra.Command, args []string) error {
	agentID, _ := cmd.Flags().GetString("agent-id")
	root, _ := cmd.Flags().GetString("loom-root")
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

	pid, alive := daemon.CheckLock(root)
	if alive {
		return fmt.Errorf("loom already running (pid %d)", pid)
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	// Create tmux session
	if !tmux.SessionExists(cfg.Tmux.SessionName) {
		if err := tmux.CreateSession(cfg.Tmux.SessionName); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}
	}

	if err := daemon.AcquireLock(root); err != nil {
		return err
	}

	// Spawn orchestrator
	_, err = agent.Spawn(root, agent.SpawnOpts{
		Role: "orchestrator",
	})
	if err != nil {
		daemon.ReleaseLock(root)
		return fmt.Errorf("spawning orchestrator: %w", err)
	}

	// Start daemon goroutines
	d := daemon.New(root, cfg)
	if err := d.Start(); err != nil {
		daemon.ReleaseLock(root)
		return fmt.Errorf("starting daemon: %w", err)
	}

	fmt.Printf("Loom started. Session: %s. Attach with: tmux attach -t %s\n", cfg.Tmux.SessionName, cfg.Tmux.SessionName)

	// Block on signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\nShutting down...")
	d.Stop()
	daemon.ReleaseLock(root)
	fmt.Println("Loom stopped.")
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
		return fmt.Errorf("sending signal to pid %d: %w", pid, err)
	}
	fmt.Printf("Sent stop signal to loom (pid %d)\n", pid)
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
			msgs, err := mail.Read(root, e.Name(), true)
			if err == nil {
				undelivered += len(msgs)
			}
		}
	}
	fmt.Printf("Mail: %d undelivered\n", undelivered)
	return nil
}
