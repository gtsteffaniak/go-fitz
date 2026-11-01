// Command fitz-compare dumps the exact-tier outputs of the reference
// implementation (the cgo wrapper in this module) as JSON. The pure Go port
// is checked against this output by the compare gate.
//
// Usage: go run ./cmd/fitz-compare <file.pdf> [more.pdf ...]
//
// This command is reference-only. It requires cgo and will be moved into the
// separate test/reference module before the cgo backend is removed.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	fitz "github.com/gtsteffaniak/go-fitz"
)

// Result is the reference output for one document.
type Result struct {
	File     string            `json:"file"`
	Error    string            `json:"error,omitempty"`
	NumPage  int               `json:"numPage"`
	Bounds   [][4]int          `json:"bounds"`
	Metadata map[string]string `json:"metadata"`
	ToC      []fitz.Outline    `json:"toc"`
	ToCErr   string            `json:"tocErr,omitempty"`
	Links    [][]fitz.Link     `json:"links"`
}

func dump(path string) Result {
	res := Result{File: path}
	doc, err := fitz.New(path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer doc.Close()

	res.NumPage = doc.NumPage()
	res.Metadata = doc.Metadata()

	for i := 0; i < res.NumPage; i++ {
		r, err := doc.Bound(i)
		if err != nil {
			res.Error = fmt.Sprintf("bound page %d: %v", i, err)
			return res
		}
		res.Bounds = append(res.Bounds, [4]int{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y})

		links, err := doc.Links(i)
		if err != nil {
			res.Error = fmt.Sprintf("links page %d: %v", i, err)
			return res
		}
		res.Links = append(res.Links, links)
	}

	toc, err := doc.ToC()
	if err != nil {
		res.ToCErr = err.Error()
	}
	res.ToC = toc
	return res
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fitz-compare <file.pdf> [...]")
		os.Exit(2)
	}
	out := make([]Result, 0, len(os.Args)-1)
	for _, p := range os.Args[1:] {
		out = append(out, dump(p))
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
