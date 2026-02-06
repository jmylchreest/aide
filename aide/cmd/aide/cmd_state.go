package main

import (
	"fmt"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdState(dbPath string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide state [set|get|delete|list|clear|cleanup]")
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "set":
		return stateSet(backend, subargs)
	case "get":
		return stateGet(backend, subargs)
	case "delete":
		return stateDelete(backend, subargs)
	case "list":
		return stateList(backend, subargs)
	case "clear":
		return stateClear(backend, subargs)
	case "cleanup":
		return stateCleanup(backend, subargs)
	default:
		return fmt.Errorf("unknown state subcommand: %s", subcmd)
	}
}

func stateSet(b *Backend, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: aide state set KEY VALUE [--agent=AGENT_ID]")
	}

	key := args[0]
	value := args[1]
	agentID := parseFlag(args[2:], "--agent=")

	if err := b.SetState(key, value, agentID); err != nil {
		return fmt.Errorf("failed to set state: %w", err)
	}

	if agentID != "" {
		fmt.Printf("Set state [%s]: %s = %s\n", agentID, key, value)
	} else {
		fmt.Printf("Set state: %s = %s\n", key, value)
	}
	return nil
}

func stateGet(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide state get KEY [--agent=AGENT_ID]")
	}

	key := args[0]
	agentID := parseFlag(args[1:], "--agent=")

	st, err := b.GetState(key, agentID)
	if err != nil {
		if err == store.ErrNotFound {
			fmt.Println("No state found for key:", key)
			return nil
		}
		return fmt.Errorf("failed to get state: %w", err)
	}

	if st.Agent != "" {
		fmt.Printf("[%s] %s = %s\n", st.Agent, key, st.Value)
	} else {
		fmt.Printf("%s = %s\n", key, st.Value)
	}
	return nil
}

func stateDelete(b *Backend, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: aide state delete KEY [--agent=AGENT_ID]")
	}

	key := args[0]
	agentID := parseFlag(args[1:], "--agent=")

	storeKey := key
	if agentID != "" {
		storeKey = fmt.Sprintf("agent:%s:%s", agentID, key)
	}

	if err := b.DeleteState(storeKey); err != nil {
		return fmt.Errorf("failed to delete state: %w", err)
	}

	fmt.Printf("Deleted state: %s\n", key)
	return nil
}

func stateList(b *Backend, args []string) error {
	agentFilter := parseFlag(args, "--agent=")

	states, err := b.ListState(agentFilter)
	if err != nil {
		return fmt.Errorf("failed to list state: %w", err)
	}

	for _, st := range states {
		if st.Agent != "" {
			fmt.Printf("[%s] %s = %s\n", st.Agent, st.Key, st.Value)
		} else {
			fmt.Printf("%s = %s\n", st.Key, st.Value)
		}
	}
	return nil
}

func stateClear(b *Backend, args []string) error {
	agentID := parseFlag(args, "--agent=")
	all := hasFlag(args, "--all")

	if agentID == "" && !all {
		return fmt.Errorf("usage: aide state clear --agent=AGENT_ID (or --all)")
	}

	count, err := b.ClearState(agentID)
	if err != nil {
		return fmt.Errorf("failed to clear state: %w", err)
	}

	if agentID != "" {
		fmt.Printf("Cleared %d state entries for agent: %s\n", count, agentID)
	} else {
		fmt.Printf("Cleared %d state entries\n", count)
	}
	return nil
}

func stateCleanup(b *Backend, args []string) error {
	maxAge := 1 * time.Hour
	if dur := parseFlag(args, "--older-than="); dur != "" {
		if d, err := time.ParseDuration(dur); err == nil {
			maxAge = d
		}
	}

	count, err := b.CleanupState(maxAge)
	if err != nil {
		return fmt.Errorf("failed to cleanup state: %w", err)
	}

	fmt.Printf("Cleaned up %d stale state entries (older than %v)\n", count, maxAge)
	return nil
}
