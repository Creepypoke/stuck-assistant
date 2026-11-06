package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func ignored(n *html.Node) bool {
	switch n.Data {
	case "head", "script", "style", "nav", "footer", "header", "aside", "form", "button", "noscript", "svg":
		return true
	}
	for _, a := range n.Attr {
		if a.Key == "hidden" {
			return true
		}
	}
	return attr(n, "aria-hidden") == "true"
}
func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Type == html.ElementNode && ignored(n) {
		return ""
	}
	var s strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		s.WriteString(textOf(ch))
		if ch.Data == "br" || ch.Data == "td" || ch.Data == "th" {
			s.WriteString(" ")
		}
	}
	return s.String()
}
func Extract(path string, limit int64) ([]byte, string, []Block, []string, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, "", nil, nil, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, "", nil, nil, e
	}
	if int64(len(raw)) > limit {
		return nil, "", nil, nil, fmt.Errorf("input exceeds max_input_bytes")
	}
	var blocks []Block
	var warnings []string
	var headings []string
	add := func(kind, text string) {
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			return
		}
		if len(kind) == 2 && kind[0] == 'h' {
			level := int(kind[1] - '0')
			for len(headings) < level {
				headings = append(headings, "")
			}
			headings = append(headings[:level-1], text)
		}
		blocks = append(blocks, Block{Kind: kind, Text: text, HeadingPath: append([]string(nil), headings...)})
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt":
		if !utf8.Valid(raw) {
			return nil, "", nil, nil, fmt.Errorf("txt must be UTF-8")
		}
		for _, paragraph := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
			p := strings.TrimSpace(paragraph)
			if p == "" {
				continue
			}
			kind := "paragraph"
			if strings.HasPrefix(p, "#") {
				n := len(p) - len(strings.TrimLeft(p, "#"))
				if n <= 6 && len(p) > n && p[n] == ' ' {
					kind = fmt.Sprintf("h%d", n)
					p = strings.TrimSpace(p[n:])
				}
			}
			add(kind, p)
		}
	case ".html", ".htm":
		r, e := charset.NewReader(bytes.NewReader(raw), "")
		if e != nil {
			return nil, "", nil, nil, e
		}
		doc, e := html.Parse(r)
		if e != nil {
			return nil, "", nil, nil, e
		}
		root := doc
		best := 0
		var choose func(*html.Node)
		choose = func(n *html.Node) {
			if n.Type == html.ElementNode {
				score := 0
				switch n.Data {
				case "body":
					score = 1
				case "main":
					score = 3
				case "article":
					score = 4
				}
				if strings.Contains(attr(n, "class"), "moz-reader-content") {
					score = 5
				}
				if score > best && len(strings.TrimSpace(textOf(n))) > 0 {
					root = n
					best = score
				}
			}
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				choose(ch)
			}
		}
		choose(doc)
		var images func(*html.Node)
		images = func(n *html.Node) {
			if n.Type == html.ElementNode && ignored(n) {
				return
			}
			if n.Type == html.ElementNode && n.Data == "img" {
				warnings = append(warnings, "Image requires review: "+attr(n, "alt"))
			}
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				images(ch)
			}
		}
		images(root)
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				if ignored(n) {
					return
				}
				switch n.Data {
				case "h1", "h2", "h3", "h4", "h5", "h6", "p", "tr", "figcaption":
					add(n.Data, textOf(n))
					return
				case "li": // Keep nested list order; do not emit text twice.
					var direct strings.Builder
					for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
						if ch.Data != "ul" && ch.Data != "ol" {
							direct.WriteString(textOf(ch))
						}
					}
					add("li", direct.String())
					for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
						if ch.Data == "ul" || ch.Data == "ol" {
							walk(ch)
						}
					}
					return
				case "img":

					return
				}
			}
			if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
				add("paragraph", n.Data)
				return
			}
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
		}
		walk(root)
	default:
		return nil, "", nil, nil, fmt.Errorf("supported input extensions: txt, htm, html")
	}
	if len(blocks) == 0 {
		return nil, "", nil, nil, fmt.Errorf("no article text extracted")
	}
	var out strings.Builder
	offset := 0
	for i := range blocks {
		if i > 0 {
			out.WriteString("\n\n")
			offset += 2
		}
		b := &blocks[i]
		b.ID = fmt.Sprintf("block-%05d", i+1)
		b.Start = offset
		out.WriteString(b.Text)
		offset += utf8.RuneCountInString(b.Text)
		b.End = offset
	}
	return raw, out.String(), blocks, warnings, nil
}
