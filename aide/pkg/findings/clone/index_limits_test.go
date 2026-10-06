package clone

import "testing"

func TestCloneBucketCapPreservesEligibleGroups(t *testing.T) {
	index := NewCloneIndex()
	index.MaxBucketSize = 2
	for _, file := range []string{"a.go", "b.go"} {
		index.AddFile(file, []HashEntry{{Hash: 1}, {Hash: 2}}, "go", nil)
	}
	if got := index.ClonePairs(50, true); len(got.Groups) != 2 || got.BucketsSkipped != 0 {
		t.Fatalf("at cap: %+v", got)
	}
	for i := 0; i < 100; i++ {
		index.AddFile("boilerplate.go", []HashEntry{{Hash: 1, TokenIdx: i * 50}}, "go", nil)
	}
	got := index.ClonePairs(50, true)
	if len(got.Groups) != 1 || got.Groups[0].Hash != 2 || got.BucketsSkipped != 1 {
		t.Fatalf("over cap changed eligible group: %+v", got)
	}
	if retained := len(index.entries[1]); retained != 0 {
		t.Fatalf("retained %d discarded boilerplate locations", retained)
	}
}

func TestCloneBucketUnlimited(t *testing.T) {
	index := NewCloneIndex()
	for i := 0; i < 1000; i++ {
		index.AddFile("same.go", []HashEntry{{Hash: 1, TokenIdx: i * 50}}, "go", nil)
	}
	got := index.ClonePairs(50, false)
	if got.BucketsSkipped != 0 || len(got.Groups) != 1 || len(got.Groups[0].Locations) != 1000 {
		t.Fatal("unlimited bucket was capped")
	}
}
