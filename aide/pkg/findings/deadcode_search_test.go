package findings

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestBytesIndexMatchesSearchContract(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		body := make([]byte, r.Intn(300))
		_, _ = r.Read(body)
		needle := make([]byte, r.Intn(20))
		_, _ = r.Read(needle)
		if len(body) > len(needle) && i%2 == 0 {
			copy(body[r.Intn(len(body)-len(needle)):], needle)
		}
		want := bytes.Index(body, needle)
		if len(needle) == 0 {
			want = -1
		}
		if got := bytesIndex(body, needle); got != want {
			t.Fatalf("got %d want %d for %x/%x", got, want, body, needle)
		}
	}
	for _, tc := range []struct {
		body, name string
		want       bool
	}{
		{"Foo", "Foo", true}, {"MyFoo FooBar", "Foo", false}, {"FooBar; Foo()", "Foo", true}, {"a_Foo", "Foo", false}, {"", "", false}, {"foo", "", false}, {"\x00Foo\xff", "Foo", true}, {"xx aaaab", "aaab", false},
	} {
		if got := containsToken([]byte(tc.body), []byte(tc.name)); got != tc.want {
			t.Fatalf("%q/%q got %v", tc.body, tc.name, got)
		}
	}
}

func BenchmarkDeadcodeTokenSearch(b *testing.B) {
	body := bytes.Repeat([]byte("static void worker(struct item *p) { use(p); }\n"), 2000)
	for _, name := range []string{"missing_symbol", "worker", "worker_not_present"} {
		b.Run(name, func(b *testing.B) {
			needle := []byte(name)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				_ = containsToken(body, needle)
			}
		})
	}
}
