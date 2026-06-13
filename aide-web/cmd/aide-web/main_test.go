package main

import "testing"

func TestDisplayURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		port int
		want string
	}{
		{"loopback kept", "127.0.0.1", 8080, "http://127.0.0.1:8080"},
		{"wildcard v4 to localhost", "0.0.0.0", 8080, "http://localhost:8080"},
		{"empty to localhost", "", 3000, "http://localhost:3000"},
		{"wildcard v6 to localhost", "::", 8080, "http://localhost:8080"},
		{"bracketed v6 wildcard to localhost", "[::]", 8080, "http://localhost:8080"},
		{"explicit host kept", "192.168.1.5", 9000, "http://192.168.1.5:9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayURL(tt.addr, tt.port); got != tt.want {
				t.Errorf("displayURL(%q, %d) = %q, want %q", tt.addr, tt.port, got, tt.want)
			}
		})
	}
}
