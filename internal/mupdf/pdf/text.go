package pdf

import "unicode/utf16"

// pdfDocEncoding maps the PDFDocEncoding bytes 0x80–0xA0 to Unicode (ISO
// 32000 Annex D). Bytes outside that range that are not in the table map to
// the same code point, which matches Latin-1 for 0xA1–0xFF.
var pdfDocEncoding = [0x21]rune{
	0x2022, 0x2020, 0x2021, 0x2026, 0x2014, 0x2013, 0x0192, 0x2044, // 80–87
	0x2039, 0x203A, 0x2212, 0x2030, 0x201E, 0x201C, 0x201D, 0x2018, // 88–8F
	0x2019, 0x201A, 0x2122, 0xFB01, 0xFB02, 0x0141, 0x0152, 0x0160, // 90–97
	0x0178, 0x017D, 0x0131, 0x0142, 0x0153, 0x0161, 0x017E, 0xFFFD, // 98–9F
	0x20AC, // A0
}

// TextString decodes a PDF text string (ISO 32000 7.9.2.2) to UTF-8. A
// UTF-16BE byte order mark selects UTF-16, a UTF-8 BOM selects UTF-8, and
// anything else is PDFDocEncoding.
func TextString(s String) string {
	b := []byte(s)
	switch {
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		units := make([]uint16, 0, (len(b)-2)/2)
		for i := 2; i+1 < len(b); i += 2 {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		}
		return string(utf16.Decode(units))
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return string(b[3:])
	}

	out := make([]rune, 0, len(b))
	for _, c := range b {
		if c >= 0x80 && c <= 0xA0 {
			out = append(out, pdfDocEncoding[c-0x80])
			continue
		}
		out = append(out, rune(c))
	}
	return string(out)
}
