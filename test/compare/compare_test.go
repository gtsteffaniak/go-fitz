// Package compare is the compare gate for the pure Go PDF port. It checks the
// pure implementation against golden outputs recorded from the reference
// (cmd/fitz-compare, which uses the cgo wrapper and MuPDF 1.28.0).
//
// The gate needs no cgo. The goldens are committed, so CI can run it on every
// platform. A separate CI job regenerates the goldens with the reference and
// fails if they have drifted.
//
// Rules, from test/compare/expectations.json:
//   - "supported": the pure result must equal the golden reference.
//   - "typed_unsupported": the call must return an error wrapping
//     pdf.ErrUnsupported. If it starts succeeding, the gate fails until the
//     manifest is updated, so the manifest cannot drift silently.
package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-fitz/internal/mupdf/pdf"
)

type caseSpec struct {
	Asset  string `json:"asset"`
	Method string `json:"method"`
	Expect string `json:"expect"`
	Reason string `json:"reason"`
}

type manifest struct {
	MupdfVersion string     `json:"mupdfVersion"`
	MupdfCommit  string     `json:"mupdfCommit"`
	Methods      []string   `json:"methods"`
	Cases        []caseSpec `json:"cases"`
}

// golden mirrors the output of cmd/fitz-compare for one file.
type golden struct {
	File     string            `json:"file"`
	Error    string            `json:"error"`
	NumPage  int               `json:"numPage"`
	Bounds   [][4]int          `json:"bounds"`
	Metadata map[string]string `json:"metadata"`
}

// repoRoot is the module root relative to this package directory.
const repoRoot = "../.."

func loadManifest(t *testing.T) manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "test", "compare", "expectations.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

func loadGolden(t *testing.T, asset string) golden {
	t.Helper()
	name := filepath.Base(asset) + ".json"
	data, err := os.ReadFile(filepath.Join(repoRoot, "test", "compare", "golden", name))
	if err != nil {
		t.Fatalf("read golden for %s: %v", asset, err)
	}
	var g []golden
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("parse golden for %s: %v", asset, err)
	}
	if len(g) != 1 {
		t.Fatalf("golden for %s has %d entries, want 1", asset, len(g))
	}
	return g[0]
}

// run executes one API method on the pure backend. Methods that the port
// does not implement yet fall through to ErrUnsupported, so the gate records
// them as typed_unsupported rather than failing to compile against them.
func run(doc *pdf.Document, method string) (any, error) {
	switch method {
	case "NumPage":
		return doc.NumPages(), nil
	case "Bound":
		out := make([][4]int, doc.NumPages())
		for i := range out {
			r, err := doc.Bound(i)
			if err != nil {
				return nil, err
			}
			out[i] = rectArray(r)
		}
		return out, nil
	case "Metadata":
		return doc.Metadata(), nil
	case "Links":
		return doc.Links(0)
	case "ToC":
		return doc.ToC()
	case "Text", "HTML", "SVG", "Image", "ImageDPI", "ImagePNG":
		return nil, fmt.Errorf("%w: %s", pdf.ErrUnsupported, method)
	}
	return nil, fmt.Errorf("unknown method %q in manifest", method)
}

func rectArray(r image.Rectangle) [4]int {
	return [4]int{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y}
}

// expected extracts the golden value for a method, in the same shape run returns.
func expected(g golden, method string) any {
	switch method {
	case "NumPage":
		return g.NumPage
	case "Bound":
		return g.Bounds
	case "Metadata":
		trimmed := make(map[string]string, len(g.Metadata))
		for k, v := range g.Metadata {
			trimmed[k] = strings.TrimRight(v, "\x00")
		}
		return trimmed
	}
	return nil
}

func TestManifestCoversEveryMethod(t *testing.T) {
	m := loadManifest(t)
	seen := make(map[string]map[string]bool)
	for _, c := range m.Cases {
		if seen[c.Asset] == nil {
			seen[c.Asset] = make(map[string]bool)
		}
		seen[c.Asset][c.Method] = true
	}
	for asset, methods := range seen {
		for _, method := range m.Methods {
			if !methods[method] {
				t.Errorf("%s: no manifest case for method %s", asset, method)
			}
		}
	}
}

func TestCompareGate(t *testing.T) {
	m := loadManifest(t)

	docs := make(map[string]*pdf.Document)
	for _, c := range m.Cases {
		if _, ok := docs[c.Asset]; ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, c.Asset))
		if err != nil {
			t.Fatalf("read %s: %v", c.Asset, err)
		}
		doc, err := pdf.Open(data)
		if err != nil {
			t.Fatalf("open %s: %v", c.Asset, err)
		}
		docs[c.Asset] = doc
	}

	for _, c := range m.Cases {
		t.Run(c.Asset+"/"+c.Method, func(t *testing.T) {
			doc := docs[c.Asset]
			got, err := run(doc, c.Method)

			switch c.Expect {
			case "supported":
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", c.Method, err)
				}
				g := loadGolden(t, c.Asset)
				want := expected(g, c.Method)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s mismatch\n got: %v\nwant: %v", c.Method, got, want)
				}
			case "typed_unsupported":
				if err == nil {
					t.Fatalf("%s now succeeds; update the manifest from typed_unsupported to supported", c.Method)
				}
				if !errors.Is(err, pdf.ErrUnsupported) {
					t.Fatalf("%s: error %v does not wrap ErrUnsupported", c.Method, err)
				}
			default:
				t.Fatalf("unknown expect %q", c.Expect)
			}
		})
	}
}
