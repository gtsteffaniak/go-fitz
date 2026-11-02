// Package compare is the compare gate for the pure Go PDF port. It checks the
// pure implementation against golden outputs recorded from the reference
// (test/reference, which uses gen2brain/go-fitz, a cgo build of MuPDF).
//
// The gate needs no cgo. The goldens are committed, so CI can run it on every
// platform. A separate CI job regenerates the goldens with the reference and
// fails if they have drifted. cmd/fitz-report publishes the same results as a
// report site.
package compare

import (
	"testing"
)

// repoRoot is the module root relative to this package directory.
const repoRoot = "../.."

func TestManifestCoversEveryMethod(t *testing.T) {
	m, err := LoadManifest(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
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
	m, err := LoadManifest(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range Evaluate(repoRoot, m) {
		t.Run(r.Asset+"/"+r.Method, func(t *testing.T) {
			if !r.Green() {
				t.Fatalf("%s: status %s: %s\n got: %v\nwant: %v", r.Method, r.Status, r.Detail, r.Got, r.Want)
			}
		})
	}
}
