package pdf

import (
	"bytes"
	"strconv"
)

// isWhite reports PDF whitespace characters (ISO 32000 7.2.2).
func isWhite(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// isDelim reports PDF delimiter characters (ISO 32000 7.2.2).
func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// parser reads PDF objects from a byte slice. resolve is used only to
// follow an indirect /Length on a stream; it may be nil.
type parser struct {
	data    []byte
	pos     int
	resolve func(Ref) (Object, error)
	depth   int
}

const maxParseDepth = 256

func (p *parser) skipSpace() {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if isWhite(c) {
			p.pos++
			continue
		}
		if c == '%' {
			for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
				p.pos++
			}
			continue
		}
		return
	}
}

// token reads a regular token: a run of characters that are neither
// whitespace nor delimiters.
func (p *parser) token() []byte {
	start := p.pos
	for p.pos < len(p.data) && !isWhite(p.data[p.pos]) && !isDelim(p.data[p.pos]) {
		p.pos++
	}
	return p.data[start:p.pos]
}

// parseObject parses one direct or indirect object at the current position.
func (p *parser) parseObject() (Object, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxParseDepth {
		return nil, corrupt("object nesting too deep")
	}

	p.skipSpace()
	if p.pos >= len(p.data) {
		return nil, corrupt("unexpected end of data")
	}

	c := p.data[p.pos]
	switch {
	case c == '/':
		p.pos++
		return p.parseName(), nil
	case c == '(':
		p.pos++
		return p.parseLiteralString()
	case c == '<':
		if p.pos+1 < len(p.data) && p.data[p.pos+1] == '<' {
			p.pos += 2
			return p.parseDictBody()
		}
		p.pos++
		return p.parseHexString()
	case c == '[':
		p.pos++
		return p.parseArrayBody()
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.parseNumberOrRef(), nil
	}

	tok := p.token()
	switch string(tok) {
	case "true":
		return Bool(true), nil
	case "false":
		return Bool(false), nil
	case "null":
		return Null{}, nil
	case "":
		// A delimiter we do not handle here (e.g. ')' or '>').
		return nil, corrupt("unexpected character %q at %d", c, p.pos)
	}
	return nil, corrupt("unexpected token %q at %d", tok, p.pos)
}

// parseNumberOrRef parses an integer or real, and checks for the "n g R"
// indirect reference form when the number is a non-negative integer.
func (p *parser) parseNumberOrRef() Object {
	start := p.pos
	tok := p.token()
	if i, err := strconv.ParseInt(string(tok), 10, 64); err == nil {
		// Look ahead for "gen R".
		save := p.pos
		p.skipSpace()
		if p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			gtok := p.token()
			if g, err := strconv.ParseInt(string(gtok), 10, 64); err == nil {
				p.skipSpace()
				if p.pos < len(p.data) && p.data[p.pos] == 'R' && (p.pos+1 >= len(p.data) || isWhite(p.data[p.pos+1]) || isDelim(p.data[p.pos+1])) {
					p.pos++
					return Ref{Num: int(i), Gen: int(g)}
				}
			}
		}
		p.pos = save
		return Integer(i)
	}
	if f, err := strconv.ParseFloat(string(tok), 64); err == nil {
		return Real(f)
	}
	// MuPDF tolerates malformed numbers such as "--5" or "1.2.3" by reading
	// what it can. Reproducing that exactly is part of the repair work; for
	// now a bad number is treated as zero, matching the lenient reads.
	p.pos = start + len(tok)
	return Integer(0)
}

func (p *parser) parseName() Name {
	var out []byte
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if isWhite(c) || isDelim(c) {
			break
		}
		if c == '#' && p.pos+2 < len(p.data) {
			if v, ok := hexPair(p.data[p.pos+1], p.data[p.pos+2]); ok {
				out = append(out, v)
				p.pos += 3
				continue
			}
		}
		out = append(out, c)
		p.pos++
	}
	return Name(out)
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func hexPair(a, b byte) (byte, bool) {
	hi, ok1 := hexVal(a)
	lo, ok2 := hexVal(b)
	return hi<<4 | lo, ok1 && ok2
}

// parseLiteralString parses a (...) string with nested parentheses and the
// standard escapes. The opening '(' has already been consumed.
func (p *parser) parseLiteralString() (Object, error) {
	var out []byte
	depth := 1
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		p.pos++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return String(out), nil
			}
			out = append(out, c)
		case '\\':
			if p.pos >= len(p.data) {
				return String(out), nil
			}
			e := p.data[p.pos]
			p.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if p.pos < len(p.data) && p.data[p.pos] == '\n' {
					p.pos++
				}
			case '\n':
				// line continuation
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '7'; k++ {
						v = v*8 + int(p.data[p.pos]-'0')
						p.pos++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
	return nil, corrupt("unterminated string")
}

// parseHexString parses <...>. The '<' has already been consumed.
func (p *parser) parseHexString() (Object, error) {
	var out []byte
	var hi byte
	half := false
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		p.pos++
		if c == '>' {
			if half {
				out = append(out, hi<<4)
			}
			return String(out), nil
		}
		if isWhite(c) {
			continue
		}
		v, ok := hexVal(c)
		if !ok {
			continue
		}
		if half {
			out = append(out, hi<<4|v)
			half = false
		} else {
			hi = v
			half = true
		}
	}
	return nil, corrupt("unterminated hex string")
}

func (p *parser) parseArrayBody() (Object, error) {
	arr := Array{}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, corrupt("unterminated array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		obj, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		arr = append(arr, obj)
	}
}

func (p *parser) parseDictBody() (Object, error) {
	d := Dict{}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, corrupt("unterminated dictionary")
		}
		if p.data[p.pos] == '>' && p.pos+1 < len(p.data) && p.data[p.pos+1] == '>' {
			p.pos += 2
			return d, nil
		}
		if p.data[p.pos] != '/' {
			// Malformed key: skip one token (or one byte if it is a delimiter)
			// and continue, so the loop always makes progress.
			if len(p.token()) == 0 {
				p.pos++
			}
			continue
		}
		p.pos++
		key := p.parseName()
		p.skipSpace()
		if p.pos < len(p.data) && p.data[p.pos] == '>' {
			// Key without a value before the closing ">>": store null.
			d[key] = Null{}
			continue
		}
		val, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		d[key] = val
	}
}

// maybeStream is called after a dictionary. If the "stream" keyword follows,
// it returns a *Stream; otherwise it returns the dictionary unchanged.
func (p *parser) maybeStream(d Dict) (Object, error) {
	save := p.pos
	p.skipSpace()
	if !bytes.HasPrefix(p.data[p.pos:], []byte("stream")) {
		p.pos = save
		return d, nil
	}
	p.pos += len("stream")
	// The keyword is followed by CRLF or LF (CR alone is tolerated).
	if p.pos < len(p.data) && p.data[p.pos] == '\r' {
		p.pos++
	}
	if p.pos < len(p.data) && p.data[p.pos] == '\n' {
		p.pos++
	}
	start := p.pos

	length := -1
	switch l := d[Name("Length")].(type) {
	case Integer:
		length = int(l)
	case Ref:
		if p.resolve != nil {
			if obj, err := p.resolve(l); err == nil {
				if n, ok := obj.(Integer); ok {
					length = int(n)
				}
			}
		}
	}

	end := -1
	if length >= 0 && start+length <= len(p.data) {
		// Accept the declared length only if "endstream" really follows.
		q := start + length
		for q < len(p.data) && isWhite(p.data[q]) {
			q++
		}
		if bytes.HasPrefix(p.data[q:], []byte("endstream")) {
			end = start + length
			p.pos = q + len("endstream")
		}
	}
	if end < 0 {
		// Broken or missing /Length: scan for the keyword, as MuPDF does.
		idx := bytes.Index(p.data[start:], []byte("endstream"))
		if idx < 0 {
			return nil, corrupt("stream without endstream")
		}
		end = start + idx
		// Drop the EOL that precedes the keyword.
		if end > start && p.data[end-1] == '\n' {
			end--
		}
		if end > start && p.data[end-1] == '\r' {
			end--
		}
		p.pos = start + idx + len("endstream")
	}
	return &Stream{Dict: d, Raw: p.data[start:end]}, nil
}

// parseIndirectAt parses "num gen obj <object> endobj" at offset and returns
// the object, verifying the number matches the expected one when want >= 0.
func parseIndirectAt(data []byte, offset int, resolve func(Ref) (Object, error), want int) (Object, error) {
	if offset < 0 || offset >= len(data) {
		return nil, corrupt("object offset %d out of range", offset)
	}
	p := &parser{data: data, pos: offset, resolve: resolve}
	p.skipSpace()
	numTok := p.token()
	num, err := strconv.Atoi(string(numTok))
	if err != nil {
		return nil, corrupt("bad object number at %d", offset)
	}
	if want >= 0 && num != want {
		return nil, corrupt("object %d found at offset %d, expected %d", num, offset, want)
	}
	p.skipSpace()
	if _, gerr := strconv.Atoi(string(p.token())); gerr != nil {
		return nil, corrupt("bad generation for object %d", num)
	}
	p.skipSpace()
	if string(p.token()) != "obj" {
		return nil, corrupt("missing obj keyword for object %d", num)
	}
	obj, err := p.parseObject()
	if err != nil {
		return nil, err
	}
	if d, ok := obj.(Dict); ok {
		obj, err = p.maybeStream(d)
		if err != nil {
			return nil, err
		}
	}
	return obj, nil
}
