package main

import (
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
)

func TestBudgetStateDisabled(t *testing.T) {
	// budget <= 0 disables the cap — every take() succeeds.
	bs := &budgetState{budget: 0}
	for i := 0; i < 100; i++ {
		if !bs.take(strings.Repeat("x", 1000)) {
			t.Fatalf("disabled budget rejected take at iteration %d", i)
		}
	}
}

func TestBudgetStateDropsLowerRanked(t *testing.T) {
	item := strings.Repeat("x", 60)
	cost := code.EstimateTokensForText(item)
	// Budget fits exactly one item, not two.
	bs := &budgetState{budget: cost, remaining: cost}

	if !bs.take(item) {
		t.Fatal("first item should be kept")
	}
	if bs.take(item) {
		t.Fatal("second item should be dropped — over budget")
	}
}

func TestBudgetStateAlwaysKeepsTopItem(t *testing.T) {
	// A single item larger than the entire budget is still kept (a match beats
	// an empty injection), but it consumes the budget so nothing else fits.
	huge := strings.Repeat("x", 4000)
	bs := &budgetState{budget: 1, remaining: 1}

	if !bs.take(huge) {
		t.Fatal("top item must always be kept even when over budget")
	}
	if bs.take("small") {
		t.Fatal("subsequent item should be dropped after budget consumed")
	}
}
