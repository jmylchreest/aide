package main

import (
	"fmt"

	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdMessage(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide message [send|list|ack|clear|prune]")
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "send":
		return messageSend(backend, subargs)
	case "list":
		return messageList(backend, subargs)
	case "ack":
		return messageAck(backend, subargs)
	case "clear":
		return messageClear(backend, dbPath, subargs)
	case "prune":
		return messagePrune(backend)
	default:
		return fmt.Errorf("unknown message subcommand: %s", subcmd)
	}
}

func messageSend(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide message send CONTENT --from=AGENT [--to=AGENT] [--type=TYPE] [--ttl=SECONDS]")
	}

	content := args[0]
	from := parseFlag(args[1:], "--from=")
	to := parseFlag(args[1:], "--to=")
	msgType := parseFlag(args[1:], "--type=")
	ttlStr := parseFlag(args[1:], "--ttl=")

	if from == "" {
		return fmt.Errorf("--from is required")
	}

	ttlSeconds := 3600 // default 1 hour
	if ttlStr != "" {
		fmt.Sscanf(ttlStr, "%d", &ttlSeconds)
	}

	msg, err := b.SendMessage(from, to, content, msgType, ttlSeconds)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	if to == "" {
		fmt.Printf("Broadcast from %s (id=%d): %s\n", from, msg.ID, content)
	} else {
		fmt.Printf("Message from %s to %s (id=%d): %s\n", from, to, msg.ID, content)
	}
	return nil
}

func messageList(b *Backend, args []string) error {
	agentID := parseFlag(args, "--agent=")

	messages, err := b.ListMessages(agentID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Println("No messages")
		return nil
	}

	for _, m := range messages {
		readStatus := ""
		if len(m.ReadBy) > 0 {
			readStatus = fmt.Sprintf(" (read by %d)", len(m.ReadBy))
		}
		if m.To == "" {
			fmt.Printf("[%d] [broadcast] %s: %s%s\n", m.ID, m.From, m.Content, readStatus)
		} else {
			fmt.Printf("[%d] [%s -> %s] %s%s\n", m.ID, m.From, m.To, m.Content, readStatus)
		}
	}
	return nil
}

func messageAck(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide message ack MESSAGE_ID --agent=AGENT")
	}

	var msgID uint64
	if _, err := fmt.Sscanf(args[0], "%d", &msgID); err != nil {
		return fmt.Errorf("invalid message ID: %s", args[0])
	}

	agentID := parseFlag(args[1:], "--agent=")
	if agentID == "" {
		return fmt.Errorf("--agent is required")
	}

	if err := b.AckMessage(msgID, agentID); err != nil {
		return fmt.Errorf("failed to ack message: %w", err)
	}

	fmt.Printf("Acknowledged message %d by %s\n", msgID, agentID)
	return nil
}

// messageClear requires direct store access for destructive operations
func messageClear(b *Backend, dbPath string, args []string) error {
	agentID := parseFlag(args, "--agent=")
	all := hasFlag(args, "--all")

	if agentID == "" && !all {
		return fmt.Errorf("usage: aide message clear --agent=AGENT (or --all)")
	}

	if b.UsingGRPC() {
		return fmt.Errorf("message clear not available when daemon is running - use direct CLI access")
	}

	s, err := store.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer s.Close()

	count, err := s.ClearMessages(agentID)
	if err != nil {
		return fmt.Errorf("failed to clear messages: %w", err)
	}

	fmt.Printf("Cleared %d messages\n", count)
	return nil
}

func messagePrune(b *Backend) error {
	count, err := b.PruneMessages()
	if err != nil {
		return fmt.Errorf("failed to prune messages: %w", err)
	}

	fmt.Printf("Pruned %d expired messages\n", count)
	return nil
}
