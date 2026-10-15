package org

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, text string) *Document {
	t.Helper()
	d, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", text, err)
	}
	if d == nil {
		t.Fatalf("Parse(%q) returned nil document", text)
	}
	return d
}

func preOrder(d *Document) []*Node {
	var out []*Node
	for _, r := range d.Roots {
		out = append(out, r.Subtree()...)
	}
	return out
}

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, d *Document)
	}{
		{
			name:  "basic nesting",
			input: "* A\nbody a\n** B\nbody b\n* C\n",
			check: func(t *testing.T, d *Document) {
				if len(d.Roots) != 2 {
					t.Fatalf("roots = %d, want 2", len(d.Roots))
				}
				a, b, c := d.Roots[0], d.Roots[0].Children[0], d.Roots[1]
				if a.Title != "A" || a.Level != 1 || a.Line != 1 {
					t.Errorf("A = %+v", a)
				}
				if b.Title != "B" || b.Level != 2 || b.Line != 3 {
					t.Errorf("B = %+v", b)
				}
				if c.Title != "C" || c.Level != 1 || c.Line != 5 {
					t.Errorf("C = %+v", c)
				}
				if b.Parent != a {
					t.Error("B.Parent != A")
				}
				if c.Parent != nil {
					t.Error("C.Parent != nil")
				}
				if got := strings.Join(a.Body, "|"); got != "body a" {
					t.Errorf("A.Body = %q", got)
				}
				if len(c.Body) != 0 {
					t.Errorf("C.Body = %q, want empty", c.Body)
				}
				wantKeys := []string{"a", "a/b", "c"}
				for i, n := range []*Node{a, b, c} {
					if n.Key != wantKeys[i] {
						t.Errorf("key[%d] = %q, want %q", i, n.Key, wantKeys[i])
					}
				}
				if len(d.All) != 3 {
					t.Errorf("len(All) = %d, want 3", len(d.All))
				}
			},
		},
		{
			name:  "tags parsing",
			input: "* Title :ctx:urgent:\nbody\n* Plain\n* Weird :a@b#c%d_e:\n",
			check: func(t *testing.T, d *Document) {
				first := d.All["title"]
				if first == nil {
					t.Fatal("key `title` missing")
				}
				if first.Title != "Title" {
					t.Errorf("Title = %q", first.Title)
				}
				if got := strings.Join(first.Tags, ","); got != "ctx,urgent" {
					t.Errorf("Tags = %v", first.Tags)
				}
				if got := first.Headline(); got != "* Title :ctx:urgent:" {
					t.Errorf("Headline = %q", got)
				}
				p := d.All["plain"]
				if p == nil || len(p.Tags) != 0 {
					t.Fatalf("Plain = %+v, want no tags", p)
				}
				w := d.All["weird"]
				if w == nil || strings.Join(w.Tags, ",") != "a@b#c%d_e" {
					t.Fatalf("Weird tags wrong: %+v", w)
				}
			},
		},
		{
			name:  "property drawer with blank line",
			input: "* A\n\n:PROPERTIES:\n:id: x1\n:Note: hello world\n:END:\nbody line\n\n** B\n",
			check: func(t *testing.T, d *Document) {
				a := d.All["x1"]
				if a == nil {
					t.Fatal("key `x1` missing")
				}
				if a.Properties["ID"] != "x1" {
					t.Errorf("ID = %q", a.Properties["ID"])
				}
				if a.Properties["NOTE"] != "hello world" {
					t.Errorf("NOTE = %q", a.Properties["NOTE"])
				}
				if got := strings.Join(a.Body, "|"); got != "body line" {
					t.Errorf("A.Body = %q, want drawer and blanks excluded", got)
				}
				b := d.All["a/b"]
				if b == nil || b.Level != 2 || b.Parent != a {
					t.Fatalf("B = %+v", b)
				}
			},
		},
		{
			name:  "custom_id override",
			input: "* A\n:PROPERTIES:\n:ID: id-value\n:CUSTOM_ID: custom-value\n:END:\n* B\n:PROPERTIES:\n:ID: only-id\n:END:\n* C\n",
			check: func(t *testing.T, d *Document) {
				want := map[string]string{
					"custom-value": "A",
					"only-id":      "B",
					"c":            "C",
				}
				for key, title := range want {
					n := d.All[key]
					if n == nil || n.Title != title {
						t.Errorf("All[%q] = %+v, want node %q", key, n, title)
					}
				}
				if len(d.All) != 3 {
					t.Errorf("len(All) = %d, want 3", len(d.All))
				}
			},
		},
		{
			name:  "ancestor slug path",
			input: "* My Project!\n:PROPERTIES:\n:CUSTOM_ID: proj\n:END:\n** Sub Task\n*** Deep Node\n",
			check: func(t *testing.T, d *Document) {
				proj := d.All["proj"]
				if proj == nil {
					t.Fatal("key `proj` missing")
				}
				sub := d.All["my-project/sub-task"]
				if sub == nil || sub.Level != 2 || sub.Parent != proj {
					t.Fatalf("Sub Task = %+v, want child of proj", sub)
				}
				if d.All["my-project/sub-task/deep-node"] == nil {
					t.Error("key `my-project/sub-task/deep-node` missing")
				}
			},
		},
		{
			name:  "duplicate keys",
			input: "* Dup\n* Dup\n* Dup\n* A\n* A\n* A-1\n",
			check: func(t *testing.T, d *Document) {
				want := []string{"dup", "dup-1", "dup-2", "a", "a-1", "a-1-1"}
				nodes := preOrder(d)
				if len(nodes) != len(want) {
					t.Fatalf("node count = %d, want %d", len(nodes), len(want))
				}
				for i, n := range nodes {
					if n.Key != want[i] {
						t.Errorf("key[%d] = %q, want %q", i, n.Key, want[i])
					}
				}
				if len(d.All) != len(want) {
					t.Errorf("len(All) = %d, want %d", len(d.All), len(want))
				}
			},
		},
		{
			name:  "hash determinism",
			input: "* A\nbody\n** B\nx\n",
			check: func(t *testing.T, d *Document) {
				d2 := mustParse(t, "* A\nbody\n** B\nx\n")
				for key, n := range d.All {
					m := d2.All[key]
					if m == nil {
						t.Fatalf("re-parse lost key %q", key)
					}
					if m.ContentHash != n.ContentHash || m.SubtreeHash != n.SubtreeHash {
						t.Errorf("hash mismatch for %q", key)
					}
					if len(n.ContentHash) != 64 || len(n.SubtreeHash) != 64 {
						t.Errorf("hash not 64 hex chars for %q", key)
					}
				}
				d3 := mustParse(t, "* A\nbody\n** B\ny\n")
				if d.All["a/b"].ContentHash == d3.All["a/b"].ContentHash {
					t.Error("child content change did not change content hash")
				}
				if d.All["a"].ContentHash != d3.All["a"].ContentHash {
					t.Error("parent content hash changed without local change")
				}
				if d.All["a"].SubtreeHash == d3.All["a"].SubtreeHash {
					t.Error("parent subtree hash did not track child change")
				}
			},
		},
		{
			name:  "empty document",
			input: "\n\n  \n",
			check: func(t *testing.T, d *Document) {
				if len(d.Roots) != 0 || len(d.All) != 0 {
					t.Errorf("roots=%d all=%d, want 0/0", len(d.Roots), len(d.All))
				}
				if d2 := mustParse(t, ""); len(d2.Roots) != 0 || len(d2.All) != 0 {
					t.Errorf("empty string: roots=%d all=%d", len(d2.Roots), len(d2.All))
				}
			},
		},
		{
			name:  "text headline subtree",
			input: "* A\nl1\n** B\nl2\n*** C\nl3\n* D\n",
			check: func(t *testing.T, d *Document) {
				a := d.Roots[0]
				if got := a.Text(); got != "* A\nl1" {
					t.Errorf("A.Text = %q", got)
				}
				b := a.Children[0]
				if got := b.Text(); got != "** B\nl2" {
					t.Errorf("B.Text = %q", got)
				}
				c := b.Children[0]
				if got := c.Headline(); got != "*** C" {
					t.Errorf("C.Headline = %q", got)
				}
				dd := d.Roots[1]
				if got := dd.Text(); got != "* D" {
					t.Errorf("D.Text = %q, want headline only", got)
				}
				sub := a.Subtree()
				want := []string{"a", "a/b", "a/b/c"}
				if len(sub) != len(want) {
					t.Fatalf("A.Subtree len = %d, want %d", len(sub), len(want))
				}
				for i, n := range sub {
					if n.Key != want[i] {
						t.Errorf("Subtree[%d].Key = %q, want %q", i, n.Key, want[i])
					}
				}
				if ds := dd.Subtree(); len(ds) != 1 || ds[0] != dd {
					t.Error("D.Subtree != [D]")
				}
			},
		},
		{
			name:  "preamble ignored",
			input: "loose text\n\n* A\n",
			check: func(t *testing.T, d *Document) {
				if len(d.All) != 1 || d.All["a"] == nil {
					t.Fatalf("All = %+v, want single node `a`", d.All)
				}
				if len(d.Roots[0].Body) != 0 {
					t.Errorf("A.Body = %q, want empty", d.Roots[0].Body)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, mustParse(t, tt.input))
		})
	}
}

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Multiple   Spaces & Symbols!!", "multiple-spaces-symbols"},
		{"", "node"},
		{"!!!", "node"},
		{"MiXeD 123", "mixed-123"},
		{"--leading-trailing--", "leading-trailing"},
		{"a--b", "a-b"},
		{"ünïcode", "n-code"},
	}
	for _, tt := range tests {
		if got := Slug(tt.in); got != tt.want {
			t.Errorf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"* A\n",
		"* A\n** B\nbody\n",
		"* T :a:b:\n:PROPERTIES:\n:X: 1\n:END:\ntail\n",
		"***\n* \n*  :t:\n",
		":PROPERTIES:\n:END:\n",
		"* A\n:PROPERTIES:\n",
		"****\r\n* x :a:\r\nbody\r\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse returned error on %q: %v", s, err)
		}
		if d == nil {
			t.Fatal("Parse returned nil document")
		}
		total := 0
		for _, r := range d.Roots {
			for _, n := range r.Subtree() {
				total++
				if n.Key == "" {
					t.Fatalf("empty key at line %d", n.Line)
				}
				if n.ContentHash == "" || n.SubtreeHash == "" {
					t.Fatalf("missing hash for key %q", n.Key)
				}
				_ = n.Headline()
				_ = n.Text()
				if n.Parent != nil && n.Parent.Level >= n.Level {
					t.Fatalf("nesting violation at %q", n.Key)
				}
			}
		}
		if len(d.All) != total {
			t.Fatalf("duplicate keys: %d nodes, %d unique keys", total, len(d.All))
		}
	})
}
