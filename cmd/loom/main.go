package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/config"
	"github.com/karanagi/loom/internal/issue"
	"github.com/karanagi/loom/internal/mail"
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
