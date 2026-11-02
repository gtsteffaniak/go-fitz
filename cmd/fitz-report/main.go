// Command fitz-report runs the compare manifest against the pure Go port and
// writes the results to the report site as data/report.json.
//
// It needs no cgo. Goldens are produced separately by cmd/fitz-compare.
//
// Usage:
//
//	go run ./cmd/fitz-report                 # write the report, always exit 0
//	go run ./cmd/fitz-report -gate           # also exit 1 if any case fails
//	go run ./cmd/fitz-report -out some/dir   # write somewhere other than the site
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gtsteffaniak/go-fitz/test/compare"
)

func main() {
	root := flag.String("root", ".", "repository root")
	site := flag.String("out", "test/compare/report_site", "report site directory; data/report.json is written below it")
	gate := flag.Bool("gate", false, "exit 1 if any case is not green")
	flag.Parse()

	m, err := compare.LoadManifest(*root)
	if err != nil {
		fail(err)
	}
	results := compare.Evaluate(*root, m)
	report := compare.BuildReport(m, results, time.Now())

	dir := filepath.Join(*root, *site, "data")
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		fail(mkErr)
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fail(err)
	}
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		fail(err)
	}

	s := report.Summary
	fmt.Printf("cases: %d  pass: %d  typed_unsupported: %d  mismatch: %d  error: %d  unexpected: %d\n",
		s.Total, s.Pass, s.TypedUnsupported, s.Mismatch, s.Error, s.Unexpected)
	fmt.Printf("report: %s\n", path)

	if *gate {
		if failures := compare.Failures(results); len(failures) > 0 {
			for _, f := range failures {
				fmt.Fprintf(os.Stderr, "FAIL %s %s [%s]: %s\n", f.Asset, f.Method, f.Status, f.Detail)
			}
			os.Exit(1)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
