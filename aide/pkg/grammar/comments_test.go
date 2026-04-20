package grammar

import "testing"

func TestIsCommentLine_PackDriven(t *testing.T) {
	cases := []struct {
		line string
		lang string
		want bool
	}{
		{"# python comment", "python", true},
		{"x = 1", "python", false},
		{"-- lua comment", "lua", true},
		{"local x = 1", "lua", false},
		{"// go comment", "go", true},
		{"/* block */", "go", true},
		{"* doc continuation", "go", true},
		{"x := 1", "go", false},
		{"<!-- html comment -->", "html", true},
		{"<div>", "html", false},
	}
	for _, c := range cases {
		if got := IsCommentLine(c.line, c.lang); got != c.want {
			t.Errorf("IsCommentLine(%q, %q) = %v, want %v", c.line, c.lang, got, c.want)
		}
	}
}

func TestIsCommentLine_NoCommentsLanguage(t *testing.T) {
	// JSON declares an empty comments block — no line should be treated as a comment.
	cases := []string{
		`"key": "//value"`,
		`"key": "/* not a comment */"`,
		`"key": "TODO"`,
	}
	for _, c := range cases {
		if IsCommentLine(c, "json") {
			t.Errorf("IsCommentLine(%q, json) = true, want false", c)
		}
	}
}

func TestExtractCommentText_PackDriven(t *testing.T) {
	cases := []struct {
		line string
		lang string
		want string
	}{
		{"x = 1  # trailing comment", "python", "trailing comment"},
		{"-- lua line comment", "lua", "lua line comment"},
		{"func() // trailing", "go", "trailing"},
		{"/* block */", "go", "block"},
		{"<!-- markdown note -->", "markdown", "markdown note"},
		{"plain code", "go", ""},
		{`"key": "// not a comment"`, "json", ""},
	}
	for _, c := range cases {
		got := ExtractCommentText(c.line, c.lang)
		if got != c.want {
			t.Errorf("ExtractCommentText(%q, %q) = %q, want %q", c.line, c.lang, got, c.want)
		}
	}
}
