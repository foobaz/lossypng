package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // for image.Decode() format registration
	_ "image/jpeg"
	"image/png"
	"os"
	"path"
	"strings"
)

const (
	noConversion = iota
	grayscaleConversion
	rgbaConversion
)

func main() {
	var convertToRGBA, convertToGrayscale bool
	var quantization int
	flag.BoolVar(&convertToRGBA, "c", false, "convert image to 32-bit color")
	flag.BoolVar(&convertToGrayscale, "g", false, "convert image to grayscale")
	flag.IntVar(&quantization, "s", 10, "quantization threshold, zero is lossless")
	flag.Parse()

	var colorConversion int
	if convertToRGBA && !convertToGrayscale {
		colorConversion = rgbaConversion
	} else if convertToGrayscale && !convertToRGBA {
		colorConversion = grayscaleConversion
	}

	for _, path := range flag.Args() {
		optimizePath(path, colorConversion, quantization)
	}
}

func optimizePath(inPath string, colorConversion, quantization int) {
	// load image
	inFile, openErr := os.Open(inPath)
	if openErr != nil {
		fmt.Printf("couldn't open %v: %v\n", inPath, openErr)
		return
	}

	decoded, _, decodeErr := image.Decode(inFile)
	inFile.Close()
	if decodeErr != nil {
		fmt.Printf("couldn't decode %v: %v\n", inPath, decodeErr)
		return
	}

	// optimize image, converting colorspace if requested
	bounds := decoded.Bounds()
	optimized := decoded // update optimized variable later if color conversion is necessary
	switch colorConversion {
	case grayscaleConversion:
		converted := image.NewGray(bounds)
		draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
		optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, quantization, 1)
		optimized = converted
	case rgbaConversion:
		converted := image.NewRGBA(bounds)
		draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
		optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, quantization, 4)
		optimized = converted
	default:
		// no color conversion requested
		switch optimizee := decoded.(type) {
		case *image.Alpha:
			optimizeForAverageFilter(optimizee.Pix, bounds, optimizee.Stride, quantization, 1)
		case *image.Gray:
			optimizeForAverageFilter(optimizee.Pix, bounds, optimizee.Stride, quantization, 1)
		case *image.RGBA:
			// most PNGs decode as image.RGBA
			optimizeForAverageFilter(optimizee.Pix, bounds, optimizee.Stride, quantization, 4)
		case *image.Paletted:
			// many PNGs decode as image.Paletted
			// use alternative paeth optimizer for paletted images
			optimizeForPaethFilter(optimizee.Pix, bounds, optimizee.Stride, quantization, optimizee.Palette)
		case *image.Alpha16:
			converted := image.NewAlpha(bounds)
			draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
			optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, quantization, 1)
			optimized = converted
		case *image.Gray16:
			converted := image.NewGray(bounds)
			draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
			optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, quantization, 1)
			optimized = converted
		default:
			// convert all other formats to RGBA
			// most JPEGs decode as image.YCbCr
			converted := image.NewRGBA(bounds)
			draw.Draw(converted, bounds, decoded, image.ZP, draw.Src)
			optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, quantization, 4)
			optimized = converted
		}
	}

	// save optimized image
	outPath := pathWithSuffix(inPath, "-lossy")
	outFile, createErr := os.Create(outPath)
	if createErr != nil {
		fmt.Printf("couldn't create %v: %v\n", outPath, createErr)
		return
	}

	encodeErr := png.Encode(outFile, optimized)
	outFile.Close()
	if encodeErr != nil {
		fmt.Printf("couldn't encode %v: %v\n", inPath, encodeErr)
		return
	}

	// TODO: print compression statistics
}

func pathWithSuffix(filePath string, suffix string) string {
	extension := path.Ext(filePath)
	insertion := len(extension)
	if insertion > 0 {
		// if extension exists, trim it off of the base filename
		insertion = strings.LastIndex(filePath, extension)
	} else {
		insertion = len(filePath)
	}
	return filePath[:insertion] + suffix + ".png"
}

func optimizeForAverageFilter(
	pixels []uint8,
	bounds image.Rectangle,
	stride, quantization, bytesPerPixel int,
) {
	if quantization <= 0 {
		// Algorithm requires positive number.
		// Zero means lossless operation, so leaving image unchanged is correct.
		// Negative number is meaningless.
		return
	}

	height := bounds.Dy()
	width := bounds.Dx()
	halfStep := (quantization + 1) / 2

	for y := 1; y < height; y++ {
		for x := 1; x < width; x++ {
			for c := 0; c < bytesPerPixel; c++ {
				offset := y*stride + x*bytesPerPixel + c
				here := int(pixels[offset])
				
				up := int(pixels[offset-stride])
				left := int(pixels[offset-bytesPerPixel])
				average := (up + left) / 2 // PNG average filter

				var newValue int
				if abs(average-here) <= quantization {
					newValue = average
				} else {
					i := (here - average) % quantization
					if i < halfStep {
						newValue = here - i
						if newValue < 0 {
							newValue = 0
						}
					} else {
						newValue = here + quantization - i
						if newValue > 255 {
							newValue = 255
						}
					}
				}
				pixels[offset] = uint8(newValue)
			}
		}
	}
}

func optimizeForPaethFilter(
	pixels []uint8,
	bounds image.Rectangle,
	stride, quantization int,
	palette color.Palette,
) {
	height := bounds.Dy()
	width := bounds.Dx()

	for y := 1; y < height; y++ {
		for x := 1; x < width; x++ {
			offset := y*stride + x
			here := pixels[offset]
			up := pixels[offset-stride]
			left := pixels[offset-1]
			diagonal := pixels[offset-stride-1]
			paeth := paethPredictor(left, up, diagonal); // PNG Paeth filter

			distance := colorDistance(palette[here], palette[paeth])
			if distance < quantization {
				pixels[offset] = paeth
			}
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

func colorDistance(a, b color.Color) int {
	const componentCount = 4
	var ca, cb [componentCount]uint32
	ca[0], ca[1], ca[2], ca[3] = a.RGBA()
	cb[0], cb[1], cb[2], cb[3] = b.RGBA()
	var d2 uint64
	for i := 0; i < componentCount; i++ {
		var d uint32
		if ca[i] > cb[i] {
			d = ca[i] - cb[i]
		} else {
			d = cb[i] - ca[i]
		}
		d2 += uint64(d * d)
	}

	// ca/cb components are in 16-bit color, output must be 8-bit color, so shift
	return int(gapsqrt64(d2) >> 8)
}

func gapsqrt64(x uint64) uint32 {
	var rem, root uint64
	for i := 0; i < 32; i++ {
		root <<= 1
		rem = (rem << 2) | (x >> 62)
		x <<= 2
		if root < rem {
			rem -= root | 1
			root += 2
		}
	}
	return uint32(root >> 1)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}

	return x
}
