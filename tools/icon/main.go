package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/leaanthony/winicon"
	"golang.org/x/image/draw"
)

const (
	size  = 1024
	scale = 4
)

type point struct{ x, y float64 }

func insideRoundedRect(x, y, left, top, right, bottom, radius float64) bool {
	cx := math.Max(left+radius, math.Min(x, right-radius))
	cy := math.Max(top+radius, math.Min(y, bottom-radius))
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

func segmentDistance(px, py float64, a, b point) float64 {
	vx, vy := b.x-a.x, b.y-a.y
	wx, wy := px-a.x, py-a.y
	length := vx*vx + vy*vy
	t := 0.0
	if length > 0 { t = math.Max(0, math.Min(1, (wx*vx+wy*vy)/length)) }
	dx, dy := px-(a.x+t*vx), py-(a.y+t*vy)
	return math.Sqrt(dx*dx + dy*dy)
}

func makeIcon() image.Image {
	large := image.NewNRGBA(image.Rect(0, 0, size*scale, size*scale))
	coral := color.NRGBA{R: 228, G: 93, B: 72, A: 255}
	white := color.NRGBA{R: 255, G: 249, B: 247, A: 255}
	left, top, right, bottom, radius := 82.0*scale, 82.0*scale, 942.0*scale, 942.0*scale, 205.0*scale
	for y := 0; y < size*scale; y++ {
		for x := 0; x < size*scale; x++ {
			if insideRoundedRect(float64(x), float64(y), left, top, right, bottom, radius) {
				large.SetNRGBA(x, y, coral)
			}
		}
	}
	a := point{302 * scale, 520 * scale}
	b := point{448 * scale, 656 * scale}
	c := point{736 * scale, 360 * scale}
	halfWidth := 43.0 * scale
	for y := int(300 * scale); y < int(720*scale); y++ {
		for x := int(240 * scale); x < int(800*scale); x++ {
			if math.Min(segmentDistance(float64(x), float64(y), a, b), segmentDistance(float64(x), float64(y), b, c)) <= halfWidth {
				large.SetNRGBA(x, y, white)
			}
		}
	}
	result := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(result, result.Bounds(), large, large.Bounds(), draw.Over, nil)
	return result
}

func main() {
	icon := makeIcon()
	pngFile, err := os.Create("build/appicon.png")
	if err != nil { log.Fatal(err) }
	if err := png.Encode(pngFile, icon); err != nil { log.Fatal(err) }
	if err := pngFile.Close(); err != nil { log.Fatal(err) }

	input, err := os.Open("build/appicon.png")
	if err != nil { log.Fatal(err) }
	defer input.Close()
	output, err := os.Create("build/windows/icon.ico")
	if err != nil { log.Fatal(err) }
	defer output.Close()
	if err := winicon.GenerateIcon(input, output, []int{16, 24, 32, 48, 64, 128, 256}); err != nil { log.Fatal(err) }
}
