package pdf

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func openFixture(t *testing.T, name string) *Document {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	doc, err := Open(data)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	return doc
}

// TestTestPDFStructure checks the structure layer against the reference
// output recorded from the cgo wrapper (cmd/fitz-compare on testdata/test.pdf).
func TestTestPDFStructure(t *testing.T) {
	doc := openFixture(t, "test.pdf")

	if got := doc.Version(); got != "1.5" {
		t.Errorf("version = %q, want 1.5", got)
	}
	if got := doc.NumPages(); got != 84 {
		t.Fatalf("NumPages = %d, want 84", got)
	}
	for i := 0; i < doc.NumPages(); i++ {
		p, _ := doc.page(i)
		r := pageCropBox(p)
		got := [4]int{int(r.X0), int(r.Y0), int(r.X1), int(r.Y1)}
		if got != [4]int{0, 0, 612, 792} {
			t.Errorf("page %d bound = %v, want [0 0 612 792]", i, got)
		}
	}
}

// TestTestPDFInfo checks the Info dictionary. The reference wrapper pads
// values with NULs to 256 bytes; the port returns trimmed values, so the
// expectations here are the trimmed reference values.
func TestTestPDFInfo(t *testing.T) {
	doc := openFixture(t, "test.pdf")
	info := doc.Info()

	want := map[string]string{
		"Creator":      "LaTeX with hyperref package",
		"CreationDate": "D:20160711202637-07'00'",
		"Producer":     "xdvipdfmx (20140317)",
	}
	for k, v := range want {
		if got := info[k]; got != v {
			t.Errorf("Info[%s] = %q, want %q", k, got, v)
		}
	}
	for k, v := range info {
		if strings.ContainsRune(v, 0) {
			t.Errorf("Info[%s] contains NUL bytes: %q", k, v)
		}
	}
}

func TestNotPDF(t *testing.T) {
	if _, err := Open([]byte("hello world, not a pdf")); err == nil {
		t.Fatal("expected error for non-PDF input")
	}
}

// buildPDF assembles a classic-xref PDF from object bodies (object 1 is the
// catalog). Offsets and startxref are computed from the bytes written, so the
// fixture cannot drift from the layout.
func buildPDF(t *testing.T, trailerExtra string, objs ...string) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj %s endobj\n", i+1, body)
	}
	xrefAt := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer << /Size %d /Root 1 0 R %s >>\n", len(objs)+1, trailerExtra)
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xrefAt)
	return []byte(b.String())
}

func TestEncryptedIsUnsupported(t *testing.T) {
	src := buildPDF(t, "/Encrypt 4 0 R",
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 200] >>",
		"<< /Filter /Standard /V 1 >>",
	)
	_, err := Open(src)
	if err == nil {
		t.Fatal("expected unsupported error for encrypted document")
	}
	if !strings.Contains(err.Error(), ErrUnsupported.Error()) {
		t.Fatalf("error = %v, want wrapping ErrUnsupported", err)
	}
}
