package fitz_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-fitz/pkg/fitz"
)

const testPDF = "testdata/test.pdf"

func TestNewFromFile(t *testing.T) {
	doc, err := fitz.New(testPDF)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = doc.Close() }()

	if n := doc.NumPage(); n != 84 {
		t.Fatalf("NumPage = %d, want 84", n)
	}
	b, err := doc.Bound(0)
	if err != nil {
		t.Fatalf("Bound(0): %v", err)
	}
	if b.Empty() {
		t.Fatalf("Bound(0) is empty: %v", b)
	}
}

func TestNewFromMemoryAndReader(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash(testPDF))
	if err != nil {
		t.Fatal(err)
	}

	fromMem, err := fitz.NewFromMemory(data)
	if err != nil {
		t.Fatalf("NewFromMemory: %v", err)
	}
	fromReader, err := fitz.NewFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewFromReader: %v", err)
	}
	if fromMem.NumPage() != fromReader.NumPage() {
		t.Fatalf("NumPage differs: memory %d, reader %d", fromMem.NumPage(), fromReader.NumPage())
	}
}

func TestOpenErrors(t *testing.T) {
	if _, err := fitz.NewFromMemory(nil); !errors.Is(err, fitz.ErrEmptyBytes) {
		t.Errorf("empty bytes: got %v, want ErrEmptyBytes", err)
	}
	if _, err := fitz.NewFromMemory([]byte("not a pdf")); !errors.Is(err, fitz.ErrNotPDF) {
		t.Errorf("non-PDF: got %v, want ErrNotPDF", err)
	}
	if _, err := fitz.New(filepath.Join("testdata", "missing.pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: got %v, want os.ErrNotExist", err)
	}
}

func TestUnportedFeaturesReportUnsupported(t *testing.T) {
	doc, err := fitz.New(testPDF)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := doc.Links(0); !errors.Is(err, fitz.ErrUnsupported) {
		t.Errorf("Links: got %v, want ErrUnsupported", err)
	}
	if _, err := doc.ToC(); !errors.Is(err, fitz.ErrUnsupported) {
		t.Errorf("ToC: got %v, want ErrUnsupported", err)
	}
}

func TestBoundOutOfRange(t *testing.T) {
	doc, err := fitz.New(testPDF)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := doc.Bound(doc.NumPage()); !errors.Is(err, fitz.ErrPageMissing) {
		t.Errorf("Bound past end: got %v, want ErrPageMissing", err)
	}
}
