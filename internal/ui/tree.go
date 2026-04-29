package ui

import (
	"html/template"
	"sort"
	"strings"
)

// FileNode is one node in a folder tree.
type FileNode struct {
	Name     string
	Path     string // full path from root
	IsDir    bool
	Children []*FileNode
}

// BuildTree turns a flat list of "a/b/c.txt"-style paths into a nested
// FileNode tree. Children are sorted: directories first, then files,
// each alphabetically.
func BuildTree(paths []string) *FileNode {
	root := &FileNode{IsDir: true}
	for _, p := range paths {
		if p == "" {
			continue
		}
		parts := strings.Split(p, "/")
		cur := root
		for i, part := range parts {
			isLeaf := i == len(parts)-1
			var child *FileNode
			for _, c := range cur.Children {
				if c.Name == part {
					child = c
					break
				}
			}
			if child == nil {
				child = &FileNode{
					Name:  part,
					Path:  strings.Join(parts[:i+1], "/"),
					IsDir: !isLeaf,
				}
				cur.Children = append(cur.Children, child)
			}
			cur = child
		}
	}
	sortTree(root)
	return root
}

func sortTree(n *FileNode) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		sortTree(c)
	}
}

// RenderTreeHTML returns the tree as a nested <ul> using Lyrebird's
// `.tree` CSS class. Each leaf is an <a class="tree-file"> linking to
// /file?path=<path>. Directory rows are not links.
func RenderTreeHTML(root *FileNode) template.HTML {
	if root == nil || len(root.Children) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul class="tree-list">`)
	for _, c := range root.Children {
		renderTreeNodeHTML(c, &b)
	}
	b.WriteString(`</ul>`)
	return template.HTML(b.String())
}

func renderTreeNodeHTML(n *FileNode, b *strings.Builder) {
	b.WriteString(`<li class="tree-node`)
	if n.IsDir {
		b.WriteString(` tree-dir`)
	} else {
		b.WriteString(` tree-file`)
	}
	b.WriteString(`">`)
	if n.IsDir {
		b.WriteString(`<span class="tree-row tree-row-dir">`)
		b.WriteString(`<span class="tree-glyph">▸</span>`)
		b.WriteString(`<span class="tree-name">`)
		b.WriteString(template.HTMLEscapeString(n.Name))
		b.WriteString(`/</span>`)
		b.WriteString(`</span>`)
		if len(n.Children) > 0 {
			b.WriteString(`<ul class="tree-list">`)
			for _, c := range n.Children {
				renderTreeNodeHTML(c, b)
			}
			b.WriteString(`</ul>`)
		}
	} else {
		b.WriteString(`<a class="tree-row tree-row-file" href="/file?path=`)
		b.WriteString(template.HTMLEscapeString(n.Path))
		b.WriteString(`">`)
		b.WriteString(`<span class="tree-glyph">·</span>`)
		b.WriteString(`<span class="tree-name">`)
		b.WriteString(template.HTMLEscapeString(n.Name))
		b.WriteString(`</span>`)
		b.WriteString(`</a>`)
	}
	b.WriteString(`</li>`)
}

// RenderTreeASCII returns the tree as box-drawing-character art:
//
//	├── README.md
//	├── fib.py
//	└── transcripts/
//	    ├── sess1.jsonl
//	    └── sess2.jsonl
//
// Used by the 8-bit travel page.
func RenderTreeASCII(root *FileNode) string {
	if root == nil {
		return ""
	}
	var b strings.Builder
	for i, c := range root.Children {
		renderASCIINode(c, "", i == len(root.Children)-1, &b)
	}
	return b.String()
}

func renderASCIINode(n *FileNode, prefix string, isLast bool, b *strings.Builder) {
	connector := "├── "
	nextPrefix := prefix + "│   "
	if isLast {
		connector = "└── "
		nextPrefix = prefix + "    "
	}
	b.WriteString(prefix)
	b.WriteString(connector)
	b.WriteString(n.Name)
	if n.IsDir {
		b.WriteString("/")
	}
	b.WriteString("\n")
	for i, c := range n.Children {
		renderASCIINode(c, nextPrefix, i == len(n.Children)-1, b)
	}
}
