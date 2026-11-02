package pdf

import (
	"math"
)

// Rect is a float32 rectangle, matching MuPDF's fz_rect layout and
// arithmetic so page bounds reproduce bit for bit.
type Rect struct {
	X0, Y0, X1, Y1 float32
}

// Matrix is an affine transform, matching fz_matrix. Components map as
// x' = a*x + c*y + e, y' = b*x + d*y + f.
type Matrix struct {
	A, B, C, D, E, F float32
}

const flt32Epsilon = float32(1.1920929e-07) // FLT_EPSILON

// isEmptyRect mirrors fz_is_empty_rect.
func isEmptyRect(r Rect) bool {
	return r.X0 >= r.X1 || r.Y0 >= r.Y1
}

// intersectRect mirrors fz_intersect_rect for finite rectangles.
func intersectRect(a, b Rect) Rect {
	if a.X0 < b.X0 {
		a.X0 = b.X0
	}
	if a.Y0 < b.Y0 {
		a.Y0 = b.Y0
	}
	if a.X1 > b.X1 {
		a.X1 = b.X1
	}
	if a.Y1 > b.Y1 {
		a.Y1 = b.Y1
	}
	return a
}

func fminf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func fmaxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// concat mirrors fz_concat: the result applies one, then two.
func concat(one, two Matrix) Matrix {
	return Matrix{
		A: one.A*two.A + one.B*two.C,
		B: one.A*two.B + one.B*two.D,
		C: one.C*two.A + one.D*two.C,
		D: one.C*two.B + one.D*two.D,
		E: one.E*two.A + one.F*two.C + two.E,
		F: one.E*two.B + one.F*two.D + two.F,
	}
}

func scaleMatrix(sx, sy float32) Matrix {
	return Matrix{A: sx, D: sy}
}

func translateMatrix(tx, ty float32) Matrix {
	return Matrix{A: 1, D: 1, E: tx, F: ty}
}

// rotateMatrix mirrors fz_rotate, including the exact 0/90/180/270 cases.
func rotateMatrix(theta float32) Matrix {
	theta = float32(math.Mod(float64(theta), 360))
	if theta < 0 {
		theta += 360
	}
	var s, c float32
	switch {
	case absf(theta) < flt32Epsilon:
		s, c = 0, 1
	case absf(90-theta) < flt32Epsilon:
		s, c = 1, 0
	case absf(180-theta) < flt32Epsilon:
		s, c = 0, -1
	case absf(270-theta) < flt32Epsilon:
		s, c = -1, 0
	default:
		rad := float64(theta) * math.Pi / 180
		s = float32(math.Sin(rad))
		c = float32(math.Cos(rad))
	}
	return Matrix{A: c, B: s, C: -s, D: c}
}

// preRotate mirrors fz_pre_rotate: rotation applied before m.
func preRotate(m Matrix, theta float32) Matrix {
	theta = float32(math.Mod(float64(theta), 360))
	if theta < 0 {
		theta += 360
	}
	switch {
	case absf(theta) < flt32Epsilon:
		return m
	case absf(90-theta) < flt32Epsilon:
		a, b := m.A, m.B
		m.A = m.C
		m.B = m.D
		m.C = -a
		m.D = -b
		return m
	case absf(180-theta) < flt32Epsilon:
		m.A, m.B, m.C, m.D = -m.A, -m.B, -m.C, -m.D
		return m
	case absf(270-theta) < flt32Epsilon:
		a, b := m.A, m.B
		m.A = -m.C
		m.B = -m.D
		m.C = a
		m.D = b
		return m
	}
	return concat(rotateMatrix(theta), m)
}

// transformRect mirrors fz_transform_rect: the bounding box of the four
// transformed corners.
func transformRect(r Rect, m Matrix) Rect {
	pts := [4][2]float32{
		{r.X0, r.Y0},
		{r.X1, r.Y0},
		{r.X0, r.Y1},
		{r.X1, r.Y1},
	}
	var out Rect
	for i, p := range pts {
		x := p[0]*m.A + p[1]*m.C + m.E
		y := p[0]*m.B + p[1]*m.D + m.F
		if i == 0 {
			out = Rect{x, y, x, y}
			continue
		}
		out.X0 = fminf(out.X0, x)
		out.Y0 = fminf(out.Y0, y)
		out.X1 = fmaxf(out.X1, x)
		out.Y1 = fmaxf(out.Y1, y)
	}
	return out
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// pageCropBox computes the box that fz_bound_page reports (FZ_CROP_BOX),
// ported from pdf_page_obj_transform_box in source/pdf/pdf-page.c. The
// returned rectangle is in top-left-origin page space.
func pageCropBox(p pageNode) Rect {
	mediabox := p.media

	// Box used for the page: CropBox clipped to MediaBox, with the MuPDF
	// fallbacks for empty or degenerate boxes.
	var usedbox Rect
	if p.crop == nil {
		usedbox = mediabox
	} else {
		usedbox = intersectRect(mediabox, *p.crop)
	}
	if isEmptyRect(usedbox) {
		usedbox = Rect{0, 0, 612, 792}
	}
	usedbox.X0 = fminf(usedbox.X0, usedbox.X1)
	usedbox.Y0 = fminf(usedbox.Y0, usedbox.Y1)
	usedbox.X1 = fmaxf(usedbox.X0, usedbox.X1)
	usedbox.Y1 = fmaxf(usedbox.Y0, usedbox.Y1)
	if usedbox.X1-usedbox.X0 < 1 || usedbox.Y1-usedbox.Y0 < 1 {
		usedbox = Rect{0, 0, 1, 1}
	}

	// Snap rotation to 0, 90, 180 or 270.
	rotate := p.rotate
	if rotate < 0 {
		rotate = 360 - ((-rotate) % 360)
	}
	if rotate >= 360 {
		rotate %= 360
	}
	rotate = 90 * ((rotate + 45) / 90)
	if rotate >= 360 {
		rotate = 0
	}

	// Map from PDF user space to top-left page space, then move the
	// CropBox origin to (0,0).
	ctm := scaleMatrix(p.userUnit, -p.userUnit)
	ctm = preRotate(ctm, float32(-rotate))

	crop := mediabox
	if p.crop != nil {
		crop = *p.crop
	}
	crop = intersectRect(crop, mediabox)
	if isEmptyRect(crop) {
		crop = Rect{0, 0, 612, 792}
	}
	crop.X0 = fminf(crop.X0, crop.X1)
	crop.Y0 = fminf(crop.Y0, crop.Y1)
	crop.X1 = fmaxf(crop.X0, crop.X1)
	crop.Y1 = fmaxf(crop.Y0, crop.Y1)
	if crop.X1-crop.X0 < 1 || crop.Y1-crop.Y0 < 1 {
		crop = Rect{0, 0, 1, 1}
	}

	crop = transformRect(crop, ctm)
	ctm = concat(ctm, translateMatrix(-crop.X0, -crop.Y0))

	return transformRect(usedbox, ctm)
}
