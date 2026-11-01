package pdf

import (
	"bytes"
	"compress/zlib"
	"io"
)

// decodeStream applies the stream's /Filter chain. Only FlateDecode (with
// optional PNG predictors) is implemented in this milestone; any other filter
// returns ErrUnsupported so the caller can report it honestly.
func decodeStream(s *Stream, resolve func(Object) Object) ([]byte, error) {
	filters := filterList(s.Dict[Name("Filter")], resolve)
	parms := parmsList(s.Dict[Name("DecodeParms")], resolve, len(filters))

	data := s.Raw
	for i, f := range filters {
		switch f {
		case "FlateDecode", "Fl":
			out, err := inflate(data)
			if err != nil {
				return nil, err
			}
			data = out
			if p := parms[i]; p != nil {
				data, err = applyPredictor(data, p, resolve)
				if err != nil {
					return nil, err
				}
			}
		default:
			return nil, unsupported("stream filter " + f)
		}
	}
	return data, nil
}

func filterList(obj Object, resolve func(Object) Object) []string {
	if obj == nil {
		return nil
	}
	obj = resolve(obj)
	switch f := obj.(type) {
	case Name:
		return []string{string(f)}
	case Array:
		var out []string
		for _, e := range f {
			if n, ok := resolve(e).(Name); ok {
				out = append(out, string(n))
			}
		}
		return out
	}
	return nil
}

// parmsList returns one decode-parameter dictionary per filter (nil when
// absent), aligned with the filter list.
func parmsList(obj Object, resolve func(Object) Object, n int) []Dict {
	out := make([]Dict, n)
	if obj == nil {
		return out
	}
	obj = resolve(obj)
	switch p := obj.(type) {
	case Dict:
		if n > 0 {
			out[0] = p
		}
	case Array:
		for i := 0; i < n && i < len(p); i++ {
			if d, ok := resolve(p[i]).(Dict); ok {
				out[i] = d
			}
		}
	}
	return out
}

// inflate decodes zlib data. A truncated or corrupt stream returns whatever
// decoded before the error, matching MuPDF's lenient behaviour.
func inflate(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, corrupt("flate header: %v", err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil && len(out) == 0 {
		return nil, corrupt("flate data: %v", err)
	}
	return out, nil
}

func intParm(d Dict, key string, def int, resolve func(Object) Object) int {
	if v, ok := resolve(d[Name(key)]).(Integer); ok {
		return int(v)
	}
	return def
}

// applyPredictor undoes a PNG predictor (/Predictor >= 10). TIFF predictor 2
// is not implemented yet.
func applyPredictor(data []byte, parms Dict, resolve func(Object) Object) ([]byte, error) {
	pred := intParm(parms, "Predictor", 1, resolve)
	if pred < 10 {
		if pred == 1 {
			return data, nil
		}
		return nil, unsupported("TIFF predictor")
	}

	colors := intParm(parms, "Colors", 1, resolve)
	bpc := intParm(parms, "BitsPerComponent", 8, resolve)
	columns := intParm(parms, "Columns", 1, resolve)
	if colors < 1 || bpc < 1 || columns < 1 {
		return nil, corrupt("bad predictor parameters")
	}

	bpp := (colors*bpc + 7) / 8
	if bpp < 1 {
		bpp = 1
	}
	rowLen := (columns*colors*bpc + 7) / 8
	prev := make([]byte, rowLen)
	out := make([]byte, 0, len(data))

	for off := 0; off < len(data); off += rowLen + 1 {
		filter := data[off]
		end := off + 1 + rowLen
		if end > len(data) {
			end = len(data)
		}
		row := make([]byte, rowLen)
		copy(row, data[off+1:end])

		for i := 0; i < rowLen; i++ {
			var left, up, upLeft byte
			if i >= bpp {
				left = row[i-bpp]
				upLeft = prev[i-bpp]
			}
			up = prev[i]
			switch filter {
			case 0:
			case 1:
				row[i] += left
			case 2:
				row[i] += up
			case 3:
				row[i] += byte((int(left) + int(up)) / 2)
			case 4:
				row[i] += paeth(left, up, upLeft)
			default:
				return nil, corrupt("bad PNG predictor type %d", filter)
			}
		}
		out = append(out, row...)
		prev = row
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := abs(p - int(a))
	pb := abs(p - int(b))
	pc := abs(p - int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
