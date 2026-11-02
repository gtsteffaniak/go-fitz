package pdf

import (
	"bytes"
	"fmt"
	"image"
	"strconv"
	"sync"
)

// Document is an opened PDF. After Open it is read-only apart from the
// object caches, which are guarded by mu, so a Document may be used from
// several goroutines at once.
type Document struct {
	data    []byte
	version string
	xref    map[int]xrefEntry
	trailer Dict
	pages   []pageNode

	mu      sync.Mutex
	objs    map[int]Object
	objStms map[int]*objStream
}

// pageNode is a leaf of the page tree with its inherited attributes resolved.
type pageNode struct {
	media    Rect
	crop     *Rect
	rotate   int
	userUnit float32
}

// Open parses a PDF held in memory. It does not repair damaged files; those
// return an error wrapping ErrCorrupt.
func Open(data []byte) (*Document, error) {
	hdr := bytes.Index(data[:min(len(data), 1024)], []byte("%PDF-"))
	if hdr < 0 {
		return nil, ErrNotPDF
	}
	// MuPDF shifts the data so offsets are relative to the header.
	data = data[hdr:]
	version := parseVersion(data)

	xref, trailer, err := readXref(data)
	if err != nil {
		return nil, err
	}

	d := &Document{
		data:    data,
		version: version,
		xref:    xref,
		trailer: trailer,
		objs:    make(map[int]Object),
		objStms: make(map[int]*objStream),
	}

	if _, ok := trailer[Name("Encrypt")]; ok {
		return nil, unsupported("encrypted documents")
	}

	root, ok := d.deref(trailer[Name("Root")]).(Dict)
	if !ok {
		return nil, corrupt("missing catalog")
	}
	pagesRoot, ok := d.deref(root[Name("Pages")]).(Dict)
	if !ok {
		return nil, corrupt("missing page tree")
	}

	w := &pageWalker{doc: d, visited: make(map[int]bool)}
	if err := w.walk(pagesRoot, inherited{rotate: 0}, 0); err != nil {
		return nil, err
	}
	d.pages = w.pages
	return d, nil
}

// parseVersion reads "x.y" after "%PDF-".
func parseVersion(data []byte) string {
	rest := data[len("%PDF-"):]
	end := 0
	for end < len(rest) && end < 8 && (rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	if end == 0 {
		return "1.4"
	}
	return string(rest[:end])
}

// Version returns the header version, for example "1.5".
func (d *Document) Version() string { return d.version }

// NumPages returns the number of leaf pages.
func (d *Document) NumPages() int { return len(d.pages) }

// Encrypted reports whether the document has an encryption dictionary. This
// milestone never returns true because encrypted documents are rejected in
// Open; the method exists so the API shape is complete.
func (d *Document) Encrypted() bool {
	_, ok := d.trailer[Name("Encrypt")]
	return ok
}

// Info returns the Info dictionary with text strings decoded to UTF-8. Keys
// are the PDF key names (Title, Author, ...). Missing keys are absent.
func (d *Document) Info() map[string]string {
	out := make(map[string]string)
	info, ok := d.deref(d.trailer[Name("Info")]).(Dict)
	if !ok {
		return out
	}
	for k, v := range info {
		switch val := d.deref(v).(type) {
		case String:
			out[string(k)] = TextString(val)
		case Name:
			out[string(k)] = string(val)
		}
	}
	return out
}

// Metadata returns the same keys as the fitz wrapper's Metadata method.
// Values are trimmed text strings; missing Info keys are empty strings.
func (d *Document) Metadata() map[string]string {
	info := d.Info()
	get := func(key string) string { return info[key] }
	enc := "None"
	if d.Encrypted() {
		enc = "Standard"
	}
	return map[string]string{
		"format":       "PDF " + d.version,
		"encryption":   enc,
		"title":        get("Title"),
		"author":       get("Author"),
		"subject":      get("Subject"),
		"keywords":     get("Keywords"),
		"creator":      get("Creator"),
		"producer":     get("Producer"),
		"creationDate": get("CreationDate"),
		"modDate":      get("ModDate"),
	}
}

// ErrPageMissing is returned for a page index outside the document.
var ErrPageMissing = fmt.Errorf("pdf: page missing")

// Bound returns the page bounds the wrapper's Bound method reports, with
// the same integer truncation.
func (d *Document) Bound(pageNumber int) (image.Rectangle, error) {
	p, ok := d.page(pageNumber)
	if !ok {
		return image.Rectangle{}, ErrPageMissing
	}
	r := pageCropBox(p)
	return image.Rect(int(r.X0), int(r.Y0), int(r.X1), int(r.Y1)), nil
}

// Links is not implemented yet: link annotation parsing and destination
// formatting are the next milestone. It reports ErrUnsupported so the compare
// gate records the case as typed_unsupported.
func (d *Document) Links(pageNumber int) ([]Link, error) {
	return nil, unsupported("links")
}

// ToC is not implemented yet: outline parsing needs the names tree and
// destination resolution. It reports ErrUnsupported.
func (d *Document) ToC() ([]Outline, error) {
	return nil, unsupported("table of contents")
}

// Link is an external link on a page.
type Link struct {
	URI string
}

// Outline is one entry of the table of contents.
type Outline struct {
	Level int
	Title string
	URI   string
	Page  int
	Top   float64
}

// Page returns the page node at index i.
func (d *Document) page(i int) (pageNode, bool) {
	if i < 0 || i >= len(d.pages) {
		return pageNode{}, false
	}
	return d.pages[i], true
}

// Object resolves an indirect reference and returns the object. Missing or
// unreadable objects resolve to Null, as PDF readers conventionally do.
func (d *Document) Object(r Ref) Object {
	d.mu.Lock()
	if o, ok := d.objs[r.Num]; ok {
		d.mu.Unlock()
		return o
	}
	d.mu.Unlock()

	// Parsing happens without holding the lock: resolving a stream /Length
	// can re-enter Object.
	obj := d.loadObject(r)

	d.mu.Lock()
	if cached, ok := d.objs[r.Num]; ok {
		d.mu.Unlock()
		return cached
	}
	d.objs[r.Num] = obj
	d.mu.Unlock()
	return obj
}

func (d *Document) loadObject(r Ref) Object {
	e, ok := d.xref[r.Num]
	if !ok {
		return Null{}
	}
	switch e.kind {
	case 'n':
		obj, err := parseIndirectAt(d.data, e.offset, d.resolveLength, r.Num)
		if err != nil {
			return Null{}
		}
		return obj
	case 'c':
		obj, err := d.objectFromStream(e.offset, e.gen, r.Num)
		if err != nil {
			return Null{}
		}
		return obj
	}
	return Null{}
}

// resolveLength is passed to the parser so an indirect /Length can be read.
func (d *Document) resolveLength(r Ref) (Object, error) {
	return d.Object(r), nil
}

// deref follows a reference once; other objects are returned unchanged.
func (d *Document) deref(o Object) Object {
	if r, ok := o.(Ref); ok {
		return d.Object(r)
	}
	return o
}

// objStream is a decoded object stream: its header maps object numbers to
// offsets within the decoded data.
type objStream struct {
	data  []byte
	first int
	nums  []int
	offs  []int
}

func (d *Document) loadObjStream(num int) (*objStream, error) {
	d.mu.Lock()
	if s, ok := d.objStms[num]; ok {
		d.mu.Unlock()
		return s, nil
	}
	d.mu.Unlock()

	obj := d.Object(Ref{Num: num})
	st, ok := obj.(*Stream)
	if !ok {
		return nil, corrupt("object stream %d missing", num)
	}
	n, _ := d.deref(st.Dict[Name("N")]).(Integer)
	first, _ := d.deref(st.Dict[Name("First")]).(Integer)
	decoded, err := decodeStream(st, d.derefFunc)
	if err != nil {
		return nil, err
	}
	if first < 0 || int(first) > len(decoded) {
		return nil, corrupt("bad /First in object stream %d", num)
	}

	s := &objStream{data: decoded, first: int(first)}
	p := &parser{data: decoded[:first]}
	for i := 0; i < int(n); i++ {
		p.skipSpace()
		on, err1 := strconv.Atoi(string(p.token()))
		p.skipSpace()
		oo, err2 := strconv.Atoi(string(p.token()))
		if err1 != nil || err2 != nil {
			break
		}
		s.nums = append(s.nums, on)
		s.offs = append(s.offs, oo)
	}

	d.mu.Lock()
	d.objStms[num] = s
	d.mu.Unlock()
	return s, nil
}

func (d *Document) objectFromStream(streamNum, index, want int) (Object, error) {
	s, err := d.loadObjStream(streamNum)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(s.nums) || s.nums[index] != want {
		// Fall back to a search by object number, as MuPDF does.
		found := -1
		for i, n := range s.nums {
			if n == want {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, corrupt("object %d not in stream %d", want, streamNum)
		}
		index = found
	}
	start := s.first + s.offs[index]
	if start < 0 || start > len(s.data) {
		return nil, corrupt("bad object offset in stream %d", streamNum)
	}
	p := &parser{data: s.data, pos: start, resolve: d.resolveLength}
	return p.parseObject()
}

func (d *Document) derefFunc(o Object) Object { return d.deref(o) }

// pageWalker visits the page tree depth-first with a cycle guard.
type pageWalker struct {
	doc     *Document
	visited map[int]bool
	pages   []pageNode
}

// inherited carries the attributes that PDF page tree nodes pass down.
type inherited struct {
	media  *Rect
	crop   *Rect
	rotate int
	hasRot bool
}

const maxPageTreeDepth = 64

func (w *pageWalker) walk(node Dict, inh inherited, depth int) error {
	if depth > maxPageTreeDepth {
		return corrupt("page tree too deep")
	}

	if r, ok := node[Name("MediaBox")]; ok {
		if rect, ok := rectFromObject(w.doc.deref(r)); ok {
			inh.media = &rect
		}
	}
	if r, ok := node[Name("CropBox")]; ok {
		if rect, ok := rectFromObject(w.doc.deref(r)); ok {
			inh.crop = &rect
		}
	}
	if r, ok := node[Name("Rotate")]; ok {
		if n, ok := w.doc.deref(r).(Integer); ok {
			inh.rotate = int(n)
			inh.hasRot = true
		}
	}

	kids, hasKids := w.doc.deref(node[Name("Kids")]).(Array)
	t, _ := w.doc.deref(node[Name("Type")]).(Name)
	if t == "Page" || (!hasKids && t != "Pages") {
		w.addLeaf(node, inh)
		return nil
	}
	if !hasKids {
		// A /Pages node with no kids contributes no pages.
		return nil
	}

	for _, kid := range kids {
		if ref, ok := kid.(Ref); ok {
			if w.visited[ref.Num] {
				continue
			}
			w.visited[ref.Num] = true
		}
		child, ok := w.doc.deref(kid).(Dict)
		if !ok {
			continue
		}
		if err := w.walk(child, inh, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (w *pageWalker) addLeaf(node Dict, inh inherited) {
	pn := pageNode{userUnit: 1}
	if inh.media != nil {
		pn.media = *inh.media
	} else {
		pn.media = Rect{0, 0, 612, 792}
	}
	pn.crop = inh.crop
	pn.rotate = inh.rotate
	switch u := w.doc.deref(node[Name("UserUnit")]).(type) {
	case Real:
		pn.userUnit = float32(u)
	case Integer:
		pn.userUnit = float32(u)
	}
	w.pages = append(w.pages, pn)
}

// rectFromObject converts a 4-element array to a Rect, normalising corner
// order is left to the caller as MuPDF does (pdf_to_rect keeps the order).
func rectFromObject(o Object) (Rect, bool) {
	arr, ok := o.(Array)
	if !ok || len(arr) < 4 {
		return Rect{}, false
	}
	var v [4]float32
	for i := 0; i < 4; i++ {
		f, ok := numberValue(arr[i])
		if !ok {
			return Rect{}, false
		}
		v[i] = f
	}
	return Rect{v[0], v[1], v[2], v[3]}, true
}

func numberValue(o Object) (float32, bool) {
	switch n := o.(type) {
	case Integer:
		return float32(n), true
	case Real:
		return float32(n), true
	}
	return 0, false
}
