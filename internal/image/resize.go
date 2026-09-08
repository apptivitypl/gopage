package image

import (
	"image"
	stdcolor "image/color"
	"image/draw"
)

func Resize(source image.Image, width int) image.Image {
	bounds := source.Bounds()
	if width <= 0 || width >= bounds.Dx() {
		return source
	}
	height := bounds.Dy() * width / bounds.Dx()
	if height < 1 {
		height = 1
	}
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	rows := sample(source, width, height)
	draw.Draw(target, target.Bounds(), rows, image.Point{}, draw.Src)
	return target
}

func sample(source image.Image, width, height int) *image.RGBA {
	bounds := source.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	scaleX := float64(bounds.Dx()) / float64(width)
	scaleY := float64(bounds.Dy()) / float64(height)
	for y := range height {
		top, bottom := span(y, scaleY, bounds.Dy())
		for x := range width {
			left, right := span(x, scaleX, bounds.Dx())
			out.SetRGBA(x, y, average(source, bounds.Min, left, top, right, bottom))
		}
	}
	return out
}

func span(index int, scale float64, limit int) (int, int) {
	start := int(float64(index) * scale)
	end := int(float64(index+1) * scale)
	if end <= start {
		end = start + 1
	}
	if end > limit {
		end = limit
	}
	return start, end
}

func average(source image.Image, origin image.Point, left, top, right, bottom int) stdcolor.RGBA {
	var red, green, blue, alpha uint64
	count := uint64((right - left) * (bottom - top))
	if count == 0 {
		return stdcolor.RGBA{}
	}
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			r, g, b, a := source.At(origin.X+x, origin.Y+y).RGBA()
			red += uint64(r)
			green += uint64(g)
			blue += uint64(b)
			alpha += uint64(a)
		}
	}
	return stdcolor.RGBA{
		R: uint8(red / count >> 8),
		G: uint8(green / count >> 8),
		B: uint8(blue / count >> 8),
		A: uint8(alpha / count >> 8),
	}
}
