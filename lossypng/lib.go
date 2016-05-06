// Package lossypng makes PNG files smaller by applying a lossy filter.
package lossypng

import (
	"image"
	"image/color"
	"image/draw"
)

// ColorConversion specifies what color profile the image should be converted
// to, if any
type ColorConversion int

const (
	// NoConversion specifies that an image should not be converted to a
	// different color profile
	NoConversion ColorConversion = iota

	// GrayscaleConversion specifies the image should be converted to grayscale
	GrayscaleConversion

	// RGBAConversion specifies the image should be converted to RGBA
	RGBAConversion
)

const deltaComponents = 4

// Compress lossyly compresses a PNG image and optionaly converts the colorspace
// of the output image. Quantization determines the strength of the compression.
// Must be >= 0 .
func Compress(
	decoded image.Image,
	colorConversion ColorConversion,
	quantization int,
) image.Image {
	// optimize image, converting colorspace if requested
	bounds := decoded.Bounds()
	optimized := decoded // update optimized variable later if color conversion is necessary
	switch colorConversion {
	case GrayscaleConversion:
		converted := image.NewGray(bounds)
		draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
		optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, 1, quantization)
		optimized = converted
	case RGBAConversion:
		converted := image.NewRGBA(bounds)
		draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
		optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, 4, quantization)
		optimized = converted
	default:
		// no color conversion requested
		switch optimizee := decoded.(type) {
		case *image.Alpha:
			optimizeForAverageFilter(optimizee.Pix, bounds, optimizee.Stride, 1, quantization)
		case *image.Gray:
			optimizeForAverageFilter(optimizee.Pix, bounds, optimizee.Stride, 1, quantization)
		case *image.NRGBA:
			optimizeForAverageFilter(optimizee.Pix, bounds, optimizee.Stride, 4, quantization)
		case *image.Paletted:
			// many PNGs decode as image.Paletted
			// use alternative paeth optimizer for paletted images
			optimizeForPaethFilter(optimizee.Pix, bounds, optimizee.Stride, quantization, optimizee.Palette)
		case *image.Alpha16:
			converted := image.NewAlpha(bounds)
			draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
			optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, 1, quantization)
			optimized = converted
		case *image.Gray16:
			converted := image.NewGray(bounds)
			draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
			optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, 1, quantization)
			optimized = converted
		default:
			// convert all other formats to RGBA
			// most JPEGs decode as image.YCbCr
			// most PNGs decode as image.RGBA
			converted := image.NewNRGBA(bounds)
			draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
			optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, 4, quantization)
			optimized = converted
		}
	}

	return optimized
}

func optimizeForAverageFilter(
	pixels []uint8,
	bounds image.Rectangle,
	stride, bytesPerPixel int,
	quantization int,
) {
	if quantization == 0 {
		// Algorithm requires positive number.
		// Zero means lossless operation, so leaving image unchanged is correct.
		return
	}

	halfStep := int32(quantization / 2)
	height := bounds.Dy()
	width := bounds.Dx()

	const errorRowCount = 3
	const filterWidth = 5
	const filterCenter = 2
	var colorError [errorRowCount][]colorDelta
	for i := 0; i < errorRowCount; i++ {
		colorError[i] = make([]colorDelta, width+filterWidth-1)
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			diffusion := diffuseColorDeltas(colorError, x+filterCenter)
			for c := 0; c < bytesPerPixel; c++ {
				offset := y*stride + x*bytesPerPixel + c
				here := int32(pixels[offset])
				var errorHere int32
				if here > 0 && here < 255 {
					var up, left int32
					if y > 0 {
						up = int32(pixels[offset-stride])
					}
					if x > 0 {
						left = int32(pixels[offset-bytesPerPixel])
					}
					average := (up + left) / 2 // PNG average filter

					newValue := diffusion[c] + here - average
					newValue += halfStep
					newValue -= newValue % int32(quantization)
					newValue += average
					if newValue >= 0 && newValue <= 255 {
						pixels[offset] = uint8(newValue)
						errorHere = here - newValue
					}
				}
				colorError[0][x+filterCenter][c] = errorHere
			}
		}
		for i := 0; i < errorRowCount; i++ {
			colorError[(i+1)%errorRowCount] = colorError[i]
		}
	}
}

func optimizeForPaethFilter(
	pixels []uint8,
	bounds image.Rectangle,
	stride int,
	quantization int,
	palette color.Palette,
) {
	colorCount := len(palette)
	if colorCount <= 0 {
		return
	}

	height := bounds.Dy()
	width := bounds.Dx()

	const errorRowCount = 3
	const filterWidth = 5
	const filterCenter = 2
	var colorError [errorRowCount][]colorDelta
	for i := 0; i < errorRowCount; i++ {
		colorError[i] = make([]colorDelta, width+filterWidth-1)
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			diffusion := diffuseColorDeltas(colorError, x+filterCenter)

			offset := y*stride + x
			here := pixels[offset]
			var up, left, diagonal uint8
			if y > 0 {
				up = pixels[offset-stride]
			}
			if x > 0 {
				left = pixels[offset-1]
			}
			if y > 0 && x > 0 {
				diagonal = pixels[offset-stride-1]
			}
			paeth := paethPredictor(left, up, diagonal) // PNG Paeth filter

			bestDelta := colorDifference(palette[here], palette[paeth])
			total := bestDelta.add(diffusion)
			var bestColor uint8
			if (total.magnitude() >> 16) < uint64(quantization*quantization) {
				bestColor = paeth
			} else {
				bestDelta = colorDifference(palette[here], palette[bestColor])
				total = bestDelta.add(diffusion)
				bestMagnitude := total.magnitude()
				for i, candidate := range palette {
					delta := colorDifference(palette[here], candidate)
					total = delta.add(diffusion)
					nextMagnitude := total.magnitude()
					if bestMagnitude > nextMagnitude {
						bestMagnitude = nextMagnitude
						bestDelta = delta
						bestColor = uint8(i)
					}
				}
			}
			pixels[offset] = bestColor
			colorError[0][x+filterCenter] = bestDelta
		}
		for i := 0; i < errorRowCount; i++ {
			colorError[(i+1)%errorRowCount] = colorError[i]
		}
	}
}

// a = left, b = above, c = upper left
func paethPredictor(a, b, c uint8) uint8 {
	// Initial estimate
	p := int(a) + int(b) - int(c)
	// Distances to a, b, c
	pa := abs(p - int(a))
	pb := abs(p - int(b))
	pc := abs(p - int(c))

	// Return nearest of a,b,c, breaking ties in order a,b,c.
	if pa <= pb && pa <= pc {
		return a
	} else if pb <= pc {
		return b
	}
	return c
}

func colorDifference(a, b color.Color) colorDelta {
	var ca, cb [4]uint32
	ca[0], ca[1], ca[2], ca[3] = a.RGBA()
	cb[0], cb[1], cb[2], cb[3] = b.RGBA()
	//fmt.Printf("ca == %v, cb == %v\n", ca, cb)

	const full = 65535
	var delta [4]int32
	for i := 0; i < 3; i++ {
		pa := ca[i] * full
		if ca[3] > 0 {
			pa /= ca[3]
		}
		pb := cb[i] * full
		if cb[3] > 0 {
			pb /= cb[3]
		}
		delta[i] = int32(pa) - int32(pb)
	}
	delta[3] = int32(ca[3]) - int32(cb[3])

	/*
	 * Compute a very basic perceptual distance using
	 * formula from http://www.compuphase.com/cmetric.htm .
	 */
	redA := ca[0]
	redB := cb[0]
	if ca[3] > 0 {
		redA = redA * full / ca[3]
	}
	if cb[3] > 0 {
		redB = redB * full / cb[3]
	}

	redMean := int32((redA + redB) / 2)
	return colorDelta{
		int32((2*full + redMean) * delta[0] / (3 * full)),
		int32(4 * delta[1] / 3),
		int32((3*full - redMean) * delta[2] / (3 * full)),
		int32(delta[3]),
	}
}

// difference between two colors in rgba
type colorDelta [deltaComponents]int32

func (a colorDelta) magnitude() uint64 {
	var d2 uint64
	for i := 0; i < deltaComponents; i++ {
		d2 += uint64(int64(a[i]) * int64(a[i]))
	}

	return d2
}

func (a colorDelta) add(b colorDelta) colorDelta {
	var delta colorDelta
	for i := 0; i < deltaComponents; i++ {
		delta[i] = a[i] + b[i]
	}
	return delta
}

func diffuseColorDeltas(colorError [3][]colorDelta, x int) colorDelta {
	var delta colorDelta
	// Sierra dithering
	for i := 0; i < deltaComponents; i++ {
		delta[i] += 2 * colorError[2][x-1][i]
		delta[i] += 3 * colorError[2][x][i]
		delta[i] += 2 * colorError[2][x+1][i]
		delta[i] += 2 * colorError[1][x-2][i]
		delta[i] += 4 * colorError[1][x-1][i]
		delta[i] += 5 * colorError[1][x][i]
		delta[i] += 4 * colorError[1][x+1][i]
		delta[i] += 2 * colorError[1][x+2][i]
		delta[i] += 3 * colorError[0][x-2][i]
		delta[i] += 5 * colorError[0][x-1][i]
		if delta[i] < 0 {
			delta[i] -= 16
		} else {
			delta[i] += 16
		}
		delta[i] /= 32
	}
	return delta
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
