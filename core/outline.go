package core

import "github.com/dh-kam/freetype-go/api"

// Outline implements api.Outline.
type Outline struct {
	Points   []api.Vector
	Tags     []byte
	Contours []int
}

func (o *Outline) GetPoints() []api.Vector {
	return o.Points
}

func (o *Outline) GetTags() []byte {
	return o.Tags
}

func (o *Outline) GetContours() []int {
	return o.Contours
}

func (o *Outline) Scale(xScale, yScale int32) {
	for i := range o.Points {
		o.Points[i].X = int32((int64(o.Points[i].X) * int64(xScale)) >> 16)
		o.Points[i].Y = int32((int64(o.Points[i].Y) * int64(yScale)) >> 16)
	}
}

func (o *Outline) Translate(x, y int32) {
	for i := range o.Points {
		o.Points[i].X += x
		o.Points[i].Y += y
	}
}

func (o *Outline) Transform(matrix *api.Matrix) {
	if matrix == nil {
		return
	}
	for i := range o.Points {
		x := o.Points[i].X
		y := o.Points[i].Y
		o.Points[i].X = int32((int64(x)*int64(matrix.XX) + int64(y)*int64(matrix.XY)) >> 16)
		o.Points[i].Y = int32((int64(x)*int64(matrix.YX) + int64(y)*int64(matrix.YY)) >> 16)
	}
}
