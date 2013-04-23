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
	"runtime"
	"strings"
	"sync"
)

const (
	noConversion = iota
	grayscaleConversion
	rgbaConversion
)

const deltaComponents = 4
type colorDelta [deltaComponents]int // difference between two colors in rgba

func main() {
	var convertToRGBA, convertToGrayscale bool
	var quantization int
	var extension string
	flag.BoolVar(&convertToRGBA, "c", false, "convert image to 32-bit color")
	flag.BoolVar(&convertToGrayscale, "g", false, "convert image to grayscale")
	flag.IntVar(&quantization, "s", 20, "quantization threshold, zero is lossless")
	flag.StringVar(&extension, "e", "-lossy.png", "filename extension of output files")
	flag.Parse()

	var colorConversion int
	if convertToRGBA && !convertToGrayscale {
		colorConversion = rgbaConversion
	} else if convertToGrayscale && !convertToRGBA {
		colorConversion = grayscaleConversion
	}

	allPaths := flag.Args()
	pathCount := len(allPaths)
	n := runtime.NumCPU()
	if n > pathCount {
		n = pathCount
	}
	if n > 1 {
		runtime.GOMAXPROCS(n)
	}
	pathChan := make(chan string)
	var waiter sync.WaitGroup
	waiter.Add(n)
	for i := 0; i < n; i++ {
		go optimizePaths(pathChan, &waiter, colorConversion, quantization, extension)
	}
	for _, path := range flag.Args() {
		pathChan <- path
	}
	close(pathChan)
	waiter.Wait()
}

func optimizePaths(
	pathChan <-chan string,
	waiter *sync.WaitGroup,
	colorConversion int,
	quantization int,
	extension string,
) {
	for path := range pathChan {
		optimizePath(path, colorConversion, quantization, extension)
	}
	waiter.Done()
}

func optimizePath(
	inPath string,
	colorConversion int,
	quantization int,
	extension string,
) {
	// load image
	inFile, openErr := os.Open(inPath)
	if openErr != nil {
		fmt.Printf("couldn't open %v: %v\n", inPath, openErr)
		return
	}

	inInfo, inStatErr := inFile.Stat()
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
		optimizeForAverageFilter(converted.Pix, bounds, converted.Stride, 1, quantization)
		optimized = converted
	case rgbaConversion:
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

	// save optimized image
	outPath := pathWithSuffix(inPath, extension)
	outFile, createErr := os.Create(outPath)
	if createErr != nil {
		fmt.Printf("couldn't create %v: %v\n", outPath, createErr)
		return
	}

	encodeErr := png.Encode(outFile, optimized)
	outInfo, outStatErr := outFile.Stat()
	outFile.Close()
	if encodeErr != nil {
		fmt.Printf("couldn't encode %v: %v\n", inPath, encodeErr)
		return
	}

	// print compression statistics
	var inSize, outSize int64
	var inSizeDesc, outSizeDesc, percentage string
	if inStatErr != nil {
		inSizeDesc = "???B"
	} else {
		inSize = inInfo.Size()
		inSizeDesc = sizeDesc(inSize)
	}
	if outStatErr != nil {
		outSizeDesc = "???B"
	} else {
		outSize = outInfo.Size()
		outSizeDesc = sizeDesc(outSize)
	}
	if inStatErr != nil || outStatErr != nil {
		percentage = "???%"
	} else {
		percentage = fmt.Sprintf("%d%%", (outSize*100+inSize/2)/inSize)
	}
	fmt.Printf(
		"compressed %s (%s) to %s (%s, %s)\n",
		path.Base(inPath),
		inSizeDesc,
		path.Base(outPath),
		outSizeDesc,
		percentage,
	)
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
	return filePath[:insertion] + suffix
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

	halfStep := quantization / 2
	height := bounds.Dy()
	width := bounds.Dx()

	// add one to make room for floyd steinberg kernel window
	colorError := make([]int, (width + 1) * bytesPerPixel)
	lastError := make([]int, (width + 1) * bytesPerPixel)

	for y := 1; y < height; y++ {
		for x := 1; x < width; x++ {
			for c := 0; c < bytesPerPixel; c++ {
				var diffusion int
				// coefficients must sum to kernelScale
				diffusion += 7 * colorError[(x-1)*bytesPerPixel + c]
				diffusion += 1 * lastError[(x-1)*bytesPerPixel + c]
				diffusion += 5 * lastError[x*bytesPerPixel + c]
				diffusion += 3 * lastError[(x+1)*bytesPerPixel + c]
				if diffusion < 0 {
					diffusion -= 8
				} else {
					diffusion += 8
				}
				diffusion /= 16

				offset := y*stride + x*bytesPerPixel + c
				here := int(pixels[offset])
				up := int(pixels[offset-stride])
				left := int(pixels[offset-bytesPerPixel])
				average := (up + left) / 2 // PNG average filter

				newValue := diffusion + here - average
				newValue += halfStep
				newValue -= newValue % quantization
				newValue += average
				var errorHere int
				if newValue >= 0 && newValue <= 255 {
					pixels[offset] = uint8(newValue)
					errorHere = here - newValue
				}
				colorError[x*bytesPerPixel + c] = errorHere
			}
		}
		lastError, colorError = colorError, lastError
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

	// add one to make room for floyd steinberg kernel window
	colorError := make([]colorDelta, width + 1)
	lastError := make([]colorDelta, width + 1)

	for y := 1; y < height; y++ {
		for x := 1; x < width; x++ {
			leftError := colorError[x-1]
			diagonalError := lastError[x-1]
			upError := lastError[x]
			rightError := lastError[x+1]
			diffusion := diffuseColorDeltas(leftError, diagonalError, upError, rightError)

			offset := y*stride + x
			here := pixels[offset]
			up := pixels[offset-stride]
			left := pixels[offset-1]
			diagonal := pixels[offset-stride-1]
			paeth := paethPredictor(left, up, diagonal) // PNG Paeth filter

			delta := colorDifference(palette[here], palette[paeth])
			total := delta.add(diffusion)
			if total.magnitude() < quantization {
				pixels[offset] = paeth
			} else {
				var bestColor int
				delta = colorDifference(palette[here], palette[bestColor])
				total = delta.add(diffusion)
				bestMagnitude := total.magnitude()
				for i := 1; i < colorCount; i++ {
					nextDelta := colorDifference(palette[here], palette[i])
					total = nextDelta.add(diffusion)
					nextMagnitude := total.magnitude()
					if bestMagnitude > nextMagnitude {
						bestMagnitude = nextMagnitude
						delta = nextDelta
						bestColor = i
					}
				}
				pixels[offset] = uint8(bestColor)
			}
			colorError[x] = delta
		}
		lastError, colorError = colorError, lastError
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
	var ca, cb [deltaComponents]uint32
	ca[0], ca[1], ca[2], ca[3] = a.RGBA()
	cb[0], cb[1], cb[2], cb[3] = b.RGBA()

	var delta colorDelta
	for i := 0; i < deltaComponents; i++ {
		delta[i] = int(ca[i]) - int(cb[i])
	}

	return delta
}

func (delta colorDelta)magnitude() int {
	var d2 uint64
	for i := 0; i < deltaComponents; i++ {
		d2 += uint64(int64(delta[i]) * int64(delta[i]))
	}

	// delta components are in 16-bit color, output must be 8-bit color, so shift
	return int(gapsqrt64(d2) >> 8)
}

func (a colorDelta)add(b colorDelta) colorDelta {
	var delta colorDelta
	for i := 0; i < deltaComponents; i++ {
		delta[i] = a[i] + b[i]
	}
	return delta
}

func diffuseColorDeltas(left, diagonal, up, right colorDelta) colorDelta {
	var delta colorDelta
	for i := 0; i < deltaComponents; i++ {
		delta[i] += 7 * left[i]
		delta[i] += 1 * diagonal[i]
		delta[i] += 5 * up[i]
		delta[i] += 3 * right[i]
		if delta[i] < 0 {
			delta[i] -= 8
		} else {
			delta[i] += 8
		}
		delta[i] /= 16
	}
	return delta
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

func sizeDesc(size int64) string {
	suffixes := []string{"B", "kB", "MB", "GB", "TB"}
	var i int
	for i = 0; i+1 < len(suffixes); i++ {
		if size < 10000 {
			break
		}
		size = (size + 500) / 1000
	}
	return fmt.Sprintf("%d%v", size, suffixes[i])
}
