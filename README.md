## go-fitz
[![Build Status](https://github.com/gtsteffaniak/go-fitz/actions/workflows/test.yml/badge.svg)](https://github.com/gtsteffaniak/go-fitz/actions)
[![GoDoc](https://godoc.org/github.com/gtsteffaniak/go-fitz/pkg/fitz?status.svg)](https://godoc.org/github.com/gtsteffaniak/go-fitz/pkg/fitz)
[![Go Report Card](https://goreportcard.com/badge/github.com/gtsteffaniak/go-fitz?branch=main)](https://goreportcard.com/report/github.com/gtsteffaniak/go-fitz)

A pure Go PDF reader. It has no cgo, no C libraries, and no runtime library dependency.

Only the PDF features the port implements are available so far. Page count, page bounds, and document metadata work. Links and the table of contents return `ErrUnsupported`. Rendering, text, HTML, SVG, and the other document formats are not ported yet.

### Import path

The public package is `github.com/gtsteffaniak/go-fitz/pkg/fitz`. Earlier versions exposed a cgo wrapper at the module root and a `pkg/fitz` package with MuPDF bindings. Those bindings are removed, and the API now covers only what the pure Go port supports.

### Layout

* `pkg/fitz` - public pure Go API over the PDF port
* `internal/mupdf/pdf` - the pure Go PDF port
* `cmd/fitz-report` - runs the comparison gate against the golden output
* `test/compare` - comparison manifest, evaluation, and golden files
* `test/reference` - separate module that builds the cgo reference, [gen2brain/go-fitz](https://github.com/gen2brain/go-fitz), used to regenerate golden output

`test/reference` is its own Go module, so its cgo dependency never enters this module. Only `make golden` and the CI golden step build it.

### Example

```go
package main

import (
	"fmt"

	"github.com/gtsteffaniak/go-fitz/pkg/fitz"
)

func main() {
	doc, err := fitz.New("test.pdf")
	if err != nil {
		panic(err)
	}
	defer doc.Close()

	fmt.Println("pages:", doc.NumPage())

	bounds, err := doc.Bound(0)
	if err != nil {
		panic(err)
	}
	fmt.Println("first page bounds:", bounds)
}
```

### Development

* `make test` runs the unit and comparison tests without cgo
* `make compare` regenerates the report and fails on any mismatch with the golden output
* `make golden` checks the golden output against the cgo reference (needs a C compiler)
