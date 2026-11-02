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
	"time"

	"github.com/gtsteffaniak/go-fitz/internal/mupdf/pdf"
)

// Result statuses. Only StatusPass and StatusTypedUnsupported are green.
const (
	// StatusPass means a supported case matched the golden reference.
	StatusPass = "pass"
	// StatusTypedUnsupported means the case is declared unsupported and
	// returned pdf.ErrUnsupported, as the manifest expects.
	StatusTypedUnsupported = "typed_unsupported"
	// StatusMismatch means a supported case differs from the golden reference.
	StatusMismatch = "mismatch"
	// StatusError means the case failed for a reason other than a mismatch.
	StatusError = "error"
	// StatusUnexpected means an unsupported case now succeeds, so the manifest
	// must be updated before the gate passes again.
	StatusUnexpected = "unexpected"
)

// CaseSpec is one manifest entry.
type CaseSpec struct {
	Asset  string `json:"asset"`
	Method string `json:"method"`
	Expect string `json:"expect"`
	Reason string `json:"reason,omitempty"`
}

// Manifest is test/compare/expectations.json.
type Manifest struct {
	MupdfVersion string     `json:"mupdfVersion"`
	MupdfCommit  string     `json:"mupdfCommit"`
	Scope        string     `json:"scope"`
	Methods      []string   `json:"methods"`
	Cases        []CaseSpec `json:"cases"`
}

// Golden mirrors the output of cmd/fitz-compare for one file.
type Golden struct {
	File     string            `json:"file"`
	Error    string            `json:"error"`
	NumPage  int               `json:"numPage"`
	Bounds   [][4]int          `json:"bounds"`
	Metadata map[string]string `json:"metadata"`
}

// Result is the outcome of one manifest case.
type Result struct {
	Asset  string `json:"asset"`
	Method string `json:"method"`
	Expect string `json:"expect"`
	Reason string `json:"reason,omitempty"`
	Status string `json:"status"`
	Got    any    `json:"got,omitempty"`
	Want   any    `json:"want,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Green reports whether the result passes the gate.
func (r Result) Green() bool {
	return r.Status == StatusPass || r.Status == StatusTypedUnsupported
}

// Summary counts results by status.
type Summary struct {
	Total            int  `json:"total"`
	Pass             int  `json:"pass"`
	TypedUnsupported int  `json:"typedUnsupported"`
	Mismatch         int  `json:"mismatch"`
	Error            int  `json:"error"`
	Unexpected       int  `json:"unexpected"`
	Green            bool `json:"green"`
}

// Report is the document written to data/report.json.
type Report struct {
	Generated string   `json:"generated"`
	Mupdf     MupdfRef `json:"mupdf"`
	Summary   Summary  `json:"summary"`
	Results   []Result `json:"results"`
}

// MupdfRef identifies the reference the goldens were recorded from.
type MupdfRef struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// LoadManifest reads test/compare/expectations.json under root.
func LoadManifest(root string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(root, "test", "compare", "expectations.json"))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// LoadGolden reads the golden reference for an asset under root.
func LoadGolden(root, asset string) (Golden, error) {
	var g []Golden
	name := filepath.Base(asset) + ".json"
	data, err := os.ReadFile(filepath.Join(root, "test", "compare", "golden", name))
	if err != nil {
		return Golden{}, fmt.Errorf("read golden for %s: %w", asset, err)
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return Golden{}, fmt.Errorf("parse golden for %s: %w", asset, err)
	}
	if len(g) != 1 {
		return Golden{}, fmt.Errorf("golden for %s has %d entries, want 1", asset, len(g))
	}
	return g[0], nil
}

// Evaluate runs every manifest case against the pure backend and returns one
// result per case, in manifest order.
func Evaluate(root string, m Manifest) []Result {
	docs := make(map[string]*pdf.Document)
	docErrs := make(map[string]error)
	goldens := make(map[string]Golden)
	goldenErrs := make(map[string]error)

	results := make([]Result, 0, len(m.Cases))
	for _, c := range m.Cases {
		res := Result{Asset: c.Asset, Method: c.Method, Expect: c.Expect, Reason: c.Reason}

		if _, ok := docs[c.Asset]; !ok {
			docs[c.Asset], docErrs[c.Asset] = openAsset(root, c.Asset)
		}
		if err := docErrs[c.Asset]; err != nil {
			res.Status, res.Detail = StatusError, err.Error()
			results = append(results, res)
			continue
		}
		doc := docs[c.Asset]

		got, err := run(doc, c.Method)
		switch c.Expect {
		case "supported":
			if err != nil {
				res.Status, res.Detail = StatusError, err.Error()
				break
			}
			if _, ok := goldens[c.Asset]; !ok {
				goldens[c.Asset], goldenErrs[c.Asset] = LoadGolden(root, c.Asset)
			}
			if gErr := goldenErrs[c.Asset]; gErr != nil {
				res.Status, res.Detail = StatusError, gErr.Error()
				break
			}
			want := expected(goldens[c.Asset], c.Method)
			res.Got, res.Want = got, want
			if reflect.DeepEqual(got, want) {
				res.Status = StatusPass
			} else {
				res.Status = StatusMismatch
				res.Detail = fmt.Sprintf("%s differs from the reference", c.Method)
			}
		case "typed_unsupported":
			switch {
			case err == nil:
				res.Status = StatusUnexpected
				res.Got = got
				res.Detail = fmt.Sprintf("%s now succeeds; change the manifest to supported", c.Method)
			case errors.Is(err, pdf.ErrUnsupported):
				res.Status = StatusTypedUnsupported
				res.Detail = err.Error()
			default:
				res.Status, res.Detail = StatusError, err.Error()
			}
		default:
			res.Status = StatusError
			res.Detail = fmt.Sprintf("unknown expect %q", c.Expect)
		}
		results = append(results, res)
	}
	return results
}

// Failures returns the results that do not pass the gate.
func Failures(results []Result) []Result {
	var out []Result
	for _, r := range results {
		if !r.Green() {
			out = append(out, r)
		}
	}
	return out
}

// BuildReport assembles the report document for the given results.
func BuildReport(m Manifest, results []Result, now time.Time) Report {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			s.Pass++
		case StatusTypedUnsupported:
			s.TypedUnsupported++
		case StatusMismatch:
			s.Mismatch++
		case StatusError:
			s.Error++
		case StatusUnexpected:
			s.Unexpected++
		}
	}
	s.Green = s.Mismatch == 0 && s.Error == 0 && s.Unexpected == 0
	return Report{
		Generated: now.UTC().Format(time.RFC3339),
		Mupdf:     MupdfRef{Version: m.MupdfVersion, Commit: m.MupdfCommit},
		Summary:   s,
		Results:   results,
	}
}

func openAsset(root, asset string) (*pdf.Document, error) {
	data, err := os.ReadFile(filepath.Join(root, asset))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", asset, err)
	}
	doc, err := pdf.Open(data)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", asset, err)
	}
	return doc, nil
}

// run executes one API method on the pure backend. Methods the port does not
// implement yet return pdf.ErrUnsupported, which the manifest records as
// typed_unsupported.
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

// expected extracts the golden value for a method, in the same shape run
// returns. The reference pads metadata with NULs, so values are trimmed.
func expected(g Golden, method string) any {
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
