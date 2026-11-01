package pdf

import (
	"bytes"
	"strconv"
)

// xrefEntry is one row of the cross-reference data.
//
//	kind 'f': free object
//	kind 'n': in-file object at byte offset, with generation
//	kind 'c': object compressed in object stream offset (its number), at index gen
type xrefEntry struct {
	kind   byte
	offset int
	gen    int
}

// findStartXref returns the byte offset named by the last "startxref".
func findStartXref(data []byte) (int, error) {
	tail := data
	if len(data) > 4096 {
		tail = data[len(data)-4096:]
	}
	idx := bytes.LastIndex(tail, []byte("startxref"))
	if idx < 0 {
		return 0, corrupt("no startxref")
	}
	p := &parser{data: tail, pos: idx + len("startxref")}
	p.skipSpace()
	n, err := strconv.Atoi(string(p.token()))
	if err != nil {
		return 0, corrupt("bad startxref value")
	}
	return n, nil
}

// readXref loads the complete cross-reference data by following Prev links
// and hybrid XRefStm streams. Entries from newer sections take precedence.
// It returns the merged table and the newest trailer dictionary.
func readXref(data []byte) (map[int]xrefEntry, Dict, error) {
	start, err := findStartXref(data)
	if err != nil {
		return nil, nil, err
	}

	merged := make(map[int]xrefEntry)
	var newest Dict
	visited := make(map[int]bool)
	queue := []int{start}

	for len(queue) > 0 {
		off := queue[0]
		queue = queue[1:]
		if visited[off] {
			continue
		}
		visited[off] = true

		entries, trailer, err := readSection(data, off)
		if err != nil {
			return nil, nil, err
		}
		if newest == nil {
			newest = trailer
		}
		for num, e := range entries {
			if _, ok := merged[num]; !ok {
				merged[num] = e
			}
		}

		if x, ok := trailer[Name("XRefStm")].(Integer); ok {
			queue = append(queue, int(x))
		}
		if prev, ok := trailer[Name("Prev")].(Integer); ok {
			queue = append(queue, int(prev))
		}
	}

	if newest == nil {
		return nil, nil, corrupt("no trailer")
	}
	return merged, newest, nil
}

// readSection reads one xref table or xref stream at offset.
func readSection(data []byte, off int) (map[int]xrefEntry, Dict, error) {
	if off < 0 || off >= len(data) {
		return nil, nil, corrupt("xref offset %d out of range", off)
	}
	p := &parser{data: data, pos: off}
	p.skipSpace()
	if bytes.HasPrefix(data[p.pos:], []byte("xref")) {
		return readXrefTable(p)
	}
	return readXrefStream(data, off)
}

func readXrefTable(p *parser) (map[int]xrefEntry, Dict, error) {
	p.pos += len("xref")
	entries := make(map[int]xrefEntry)
	for {
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, nil, corrupt("unterminated xref table")
		}
		if bytes.HasPrefix(p.data[p.pos:], []byte("trailer")) {
			break
		}
		start, err := strconv.Atoi(string(p.token()))
		if err != nil {
			return nil, nil, corrupt("bad xref subsection start")
		}
		p.skipSpace()
		count, err := strconv.Atoi(string(p.token()))
		if err != nil {
			return nil, nil, corrupt("bad xref subsection count")
		}
		for i := 0; i < count; i++ {
			p.skipSpace()
			o, err1 := strconv.Atoi(string(p.token()))
			p.skipSpace()
			g, err2 := strconv.Atoi(string(p.token()))
			p.skipSpace()
			flag := p.token()
			if err1 != nil || err2 != nil || len(flag) != 1 {
				return nil, nil, corrupt("bad xref entry")
			}
			num := start + i
			switch flag[0] {
			case 'n':
				if _, ok := entries[num]; !ok {
					entries[num] = xrefEntry{kind: 'n', offset: o, gen: g}
				}
			case 'f':
				if _, ok := entries[num]; !ok {
					entries[num] = xrefEntry{kind: 'f'}
				}
			default:
				return nil, nil, corrupt("bad xref flag %q", flag)
			}
		}
	}

	p.pos += len("trailer")
	obj, err := p.parseObject()
	if err != nil {
		return nil, nil, err
	}
	trailer, ok := obj.(Dict)
	if !ok {
		return nil, nil, corrupt("trailer is not a dictionary")
	}
	return entries, trailer, nil
}

func readXrefStream(data []byte, off int) (map[int]xrefEntry, Dict, error) {
	obj, err := parseIndirectAt(data, off, nil, -1)
	if err != nil {
		return nil, nil, err
	}
	s, ok := obj.(*Stream)
	if !ok {
		return nil, nil, corrupt("xref at %d is not a stream", off)
	}
	if t, _ := s.Dict[Name("Type")].(Name); t != "XRef" {
		return nil, nil, corrupt("xref stream at %d has wrong type", off)
	}

	decoded, err := decodeStream(s, identityResolve)
	if err != nil {
		return nil, nil, err
	}

	w, ok := s.Dict[Name("W")].(Array)
	if !ok || len(w) < 3 {
		return nil, nil, corrupt("xref stream without /W")
	}
	var widths [3]int
	for i := 0; i < 3; i++ {
		v, ok := w[i].(Integer)
		if !ok || v < 0 || v > 8 {
			return nil, nil, corrupt("bad /W entry")
		}
		widths[i] = int(v)
	}
	rowLen := widths[0] + widths[1] + widths[2]
	if rowLen == 0 {
		return nil, nil, corrupt("empty /W")
	}

	size, _ := s.Dict[Name("Size")].(Integer)
	ranges := []int{0, int(size)}
	if idx, ok := s.Dict[Name("Index")].(Array); ok {
		ranges = ranges[:0]
		for _, v := range idx {
			n, ok := v.(Integer)
			if !ok {
				return nil, nil, corrupt("bad /Index")
			}
			ranges = append(ranges, int(n))
		}
	}
	if len(ranges)%2 != 0 {
		return nil, nil, corrupt("odd /Index length")
	}

	entries := make(map[int]xrefEntry)
	pos := 0
	readField := func(width, def int) int {
		if width == 0 {
			return def
		}
		v := 0
		for i := 0; i < width; i++ {
			v = v<<8 | int(decoded[pos+i])
		}
		pos += width
		return v
	}

	for r := 0; r+1 < len(ranges); r += 2 {
		first, count := ranges[r], ranges[r+1]
		for i := 0; i < count; i++ {
			if pos+rowLen > len(decoded) {
				// Truncated stream: keep what was read.
				return entries, s.Dict, nil
			}
			typ := readField(widths[0], 1)
			f2 := readField(widths[1], 0)
			f3 := readField(widths[2], 0)
			num := first + i
			if _, ok := entries[num]; ok {
				continue
			}
			switch typ {
			case 0:
				entries[num] = xrefEntry{kind: 'f'}
			case 1:
				entries[num] = xrefEntry{kind: 'n', offset: f2, gen: f3}
			case 2:
				entries[num] = xrefEntry{kind: 'c', offset: f2, gen: f3}
			default:
				// Unknown types are treated as null, per the spec.
			}
		}
	}
	return entries, s.Dict, nil
}

// identityResolve is used while decoding xref streams, which must not depend
// on other objects except for an indirect /Length, handled by the parser.
func identityResolve(o Object) Object { return o }
