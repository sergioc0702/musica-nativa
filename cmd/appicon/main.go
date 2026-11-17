package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const scale = 2

func main() {
	const size = 512
	large := image.NewRGBA(image.Rect(0, 0, size*scale, size*scale))
	drawRoundedSquare(large, 234*scale, 105*scale, color.RGBA{20, 24, 20, 255})
	drawCircle(large, 256*scale, 256*scale, 214*scale, color.RGBA{34, 42, 35, 255})
	drawCircle(large, 256*scale, 256*scale, 184*scale, color.RGBA{15, 18, 16, 255})
	drawRing(large, 256*scale, 256*scale, 156*scale, 2*scale, color.RGBA{65, 76, 64, 255})
	drawRing(large, 256*scale, 256*scale, 135*scale, 2*scale, color.RGBA{43, 53, 45, 255})
	drawRing(large, 256*scale, 256*scale, 105*scale, 2*scale, color.RGBA{58, 69, 59, 255})
	drawCircle(large, 256*scale, 256*scale, 76*scale, color.RGBA{214, 147, 111, 255})
	drawLine(large, 226*scale, 228*scale, 226*scale, 284*scale, 8*scale, color.RGBA{247, 234, 220, 255})
	drawLine(large, 226*scale, 228*scale, 286*scale, 284*scale, 8*scale, color.RGBA{247, 234, 220, 255})
	drawLine(large, 286*scale, 228*scale, 286*scale, 284*scale, 8*scale, color.RGBA{247, 234, 220, 255})
	drawCircle(large, 256*scale, 256*scale, 7*scale, color.RGBA{20, 24, 20, 255})

	icon := downsample(large, size)
	file, err := os.Create("build/appicon.png")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if err := png.Encode(file, icon); err != nil {
		panic(err)
	}
}

func drawRoundedSquare(img *image.RGBA, halfSize, radius int, fill color.RGBA) {
	center := img.Bounds().Dx() / 2
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			dx := max(abs(x-center)-halfSize+radius, 0)
			dy := max(abs(y-center)-halfSize+radius, 0)
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, fill)
			}
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, radius int, fill color.RGBA) {
	for y := max(cy-radius, 0); y < min(cy+radius, img.Bounds().Dy()); y++ {
		for x := max(cx-radius, 0); x < min(cx+radius, img.Bounds().Dx()); x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, fill)
			}
		}
	}
}

func drawRing(img *image.RGBA, cx, cy, radius, width int, fill color.RGBA) {
	outer := radius + width/2
	inner := max(radius-width/2, 0)
	for y := max(cy-outer, 0); y < min(cy+outer, img.Bounds().Dy()); y++ {
		for x := max(cx-outer, 0); x < min(cx+outer, img.Bounds().Dx()); x++ {
			dx, dy := x-cx, y-cy
			distance := dx*dx + dy*dy
			if distance <= outer*outer && distance >= inner*inner {
				img.SetRGBA(x, y, fill)
			}
		}
	}
}

func drawLine(img *image.RGBA, x1, y1, x2, y2, width int, fill color.RGBA) {
	left, right := min(x1, x2)-width, max(x1, x2)+width
	top, bottom := min(y1, y2)-width, max(y1, y2)+width
	dx, dy := float64(x2-x1), float64(y2-y1)
	lengthSquared := dx*dx + dy*dy
	for y := max(top, 0); y < min(bottom, img.Bounds().Dy()); y++ {
		for x := max(left, 0); x < min(right, img.Bounds().Dx()); x++ {
			t := ((float64(x-x1)*dx + float64(y-y1)*dy) / lengthSquared)
			t = math.Max(0, math.Min(1, t))
			nearX, nearY := float64(x1)+t*dx, float64(y1)+t*dy
			distanceX, distanceY := float64(x)-nearX, float64(y)-nearY
			if distanceX*distanceX+distanceY*distanceY <= float64(width*width)/4 {
				img.SetRGBA(x, y, fill)
			}
		}
	}
}

func downsample(source *image.RGBA, size int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var red, green, blue, alpha uint32
			for oy := 0; oy < scale; oy++ {
				for ox := 0; ox < scale; ox++ {
					pixel := source.RGBAAt(x*scale+ox, y*scale+oy)
					red += uint32(pixel.R)
					green += uint32(pixel.G)
					blue += uint32(pixel.B)
					alpha += uint32(pixel.A)
				}
			}
			count := uint32(scale * scale)
			result.SetRGBA(x, y, color.RGBA{uint8(red / count), uint8(green / count), uint8(blue / count), uint8(alpha / count)})
		}
	}
	return result
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
