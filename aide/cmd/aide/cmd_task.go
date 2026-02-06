package main

import (
	"fmt"
)

func cmdTask(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide task [create|claim|complete|list]")
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "create":
		return taskCreate(backend, subargs)
	case "claim":
		return taskClaim(backend, subargs)
	case "complete":
		return taskComplete(backend, subargs)
	case "list":
		return taskList(backend, subargs)
	default:
		return fmt.Errorf("unknown task subcommand: %s", subcmd)
	}
}

func taskCreate(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide task create TITLE [--description=DESC]")
	}

	title := args[0]
	desc := parseFlag(args[1:], "--description=")

	t, err := b.CreateTask(title, desc)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	fmt.Printf("Created task: %s\n", t.ID)
	return nil
}

func taskClaim(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide task claim TASK_ID --agent=AGENT_ID")
	}

	taskID := args[0]
	agentID := parseFlag(args[1:], "--agent=")

	if agentID == "" {
		return fmt.Errorf("--agent is required")
	}

	task, err := b.ClaimTask(taskID, agentID)
	if err != nil {
		return fmt.Errorf("failed to claim task: %w", err)
	}

	fmt.Printf("Claimed task: %s by %s\n", task.ID, agentID)
	return nil
}

func taskComplete(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide task complete TASK_ID [--result=RESULT]")
	}

	taskID := args[0]
	result := parseFlag(args[1:], "--result=")

	if err := b.CompleteTask(taskID, result); err != nil {
		return fmt.Errorf("failed to complete task: %w", err)
	}

	fmt.Printf("Completed task: %s\n", taskID)
	return nil
}

func taskList(b *Backend, args []string) error {
	status := parseFlag(args, "--status=")

	tasks, err := b.ListTasks(status)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	for _, t := range tasks {
		idDisplay := t.ID
		if len(t.ID) > 8 {
			idDisplay = t.ID[:8]
		}
		fmt.Printf("[%s] %s: %s\n", t.Status, idDisplay, t.Title)
	}
	return nil
}
