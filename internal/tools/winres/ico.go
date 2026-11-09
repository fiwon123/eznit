package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"sort"
)

// iconSizes are the resolutions Explorer asks for, smallest first. 256 is the
// large-thumbnail size and 16 the list view.
var iconSizes = []int{16, 32, 48, 64, 256}

// icon is one resolution of the application icon, already PNG-encoded.
type icon struct {
	size int
	png  []byte
}

// iconImages decodes the source PNG and re-encodes it at every size Windows
// asks for. ICO supports PNG-compressed entries since Vista, which is what lets
// one file carry a 256 px image without megabytes of raw bitmap.
func iconImages(source io.Reader) ([]icon, error) {
	decoded, err := png.Decode(source)
	if err != nil {
		return nil, fmt.Errorf("decode icon: %w", err)
	}

	images := make([]icon, 0, len(iconSizes))

	for _, size := range iconSizes {
		var encoded bytes.Buffer

		if err := png.Encode(&encoded, resize(decoded, size)); err != nil {
			return nil, fmt.Errorf("encode icon at %d: %w", size, err)
		}

		images = append(images, icon{size: size, png: encoded.Bytes()})
	}

	return images, nil
}

// resize box-filters the image to a square of the given size. Averaging the
// source pixels keeps the rounded tile and the fine strokes legible at 16 px,
// where nearest-neighbour would lose them.
func resize(source image.Image, size int) *image.NRGBA {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	target := image.NewNRGBA(image.Rect(0, 0, size, size))

	for y := 0; y < size; y++ {
		y0 := bounds.Min.Y + y*height/size
		y1 := bounds.Min.Y + max((y+1)*height/size, y*height/size+1)

		for x := 0; x < size; x++ {
			x0 := bounds.Min.X + x*width/size
			x1 := bounds.Min.X + max((x+1)*width/size, x*width/size+1)

			var red, green, blue, alpha float64

			count := 0

			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					// Average premultiplied colour: a transparent pixel would
					// otherwise darken the edge of the icon.
					r, g, b, a := source.At(sx, sy).RGBA()
					weight := float64(a) / 0xffff

					red += float64(r) / 0xffff * weight
					green += float64(g) / 0xffff * weight
					blue += float64(b) / 0xffff * weight
					alpha += weight

					count++
				}
			}

			if count == 0 || alpha == 0 {
				continue
			}

			target.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Round(math.Min(1, red/alpha) * 255)),
				G: uint8(math.Round(math.Min(1, green/alpha) * 255)),
				B: uint8(math.Round(math.Min(1, blue/alpha) * 255)),
				A: uint8(math.Round(math.Min(1, alpha/float64(count)) * 255)),
			})
		}
	}

	return target
}

// groupIcon is RT_GROUP_ICON's payload: it names the sizes and the resource ids
// the individual RT_ICON entries are stored under.
func groupIcon(images []icon) []byte {
	ordered := sortedIcons(images)

	var buffer bytes.Buffer

	binary.Write(&buffer, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buffer, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buffer, binary.LittleEndian, uint16(len(ordered)))

	for index, image := range ordered {
		dimension := iconDimension(image.size)

		buffer.WriteByte(dimension) // width
		buffer.WriteByte(dimension) // height
		buffer.WriteByte(0)         // colour count
		buffer.WriteByte(0)         // reserved

		binary.Write(&buffer, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&buffer, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&buffer, binary.LittleEndian, uint32(len(image.png)))
		binary.Write(&buffer, binary.LittleEndian, uint16(index+1)) // resource id
	}

	return buffer.Bytes()
}

func sortedIcons(images []icon) []icon {
	ordered := append([]icon(nil), images...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].size < ordered[j].size })

	return ordered
}

func iconDimension(size int) uint8 {
	if size >= 256 {
		return 0
	}

	return uint8(size)
}
