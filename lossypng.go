package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/gif" // for image.Decode() format registration
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/foobaz/lossypng/lossypng"
)

func main() {
	var convertToRGBA, convertToGrayscale, rewriteOriginal bool
	var quantization int
	var extension, addProcessing string
	flag.BoolVar(&rewriteOriginal, "r", false, "rewrite original")
	flag.BoolVar(&convertToRGBA, "c", false, "convert image to 32-bit color")
	flag.BoolVar(&convertToGrayscale, "g", false, "convert image to grayscale")
	flag.IntVar(&quantization, "s", 20, "quantization threshold, zero is lossless")
	flag.StringVar(&extension, "e", "-lossy.png", "filename extension of output files")
	flag.StringVar(&addProcessing, "a", "", "external command after fail")
	flag.Parse()

	var colorConversion lossypng.ColorConversion
	if convertToRGBA && !convertToGrayscale {
		colorConversion = lossypng.RGBAConversion
	} else if convertToGrayscale && !convertToRGBA {
		colorConversion = lossypng.GrayscaleConversion
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
		go optimizePaths(pathChan, &waiter, colorConversion, quantization, extension, rewriteOriginal, addProcessing)
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
	colorConversion lossypng.ColorConversion,
	quantization int,
	extension string,
	rewriteOriginal bool,
	addProcessing string,
) {
	for path := range pathChan {
		optimizePath(path, colorConversion, quantization, extension, rewriteOriginal, addProcessing)
	}
	waiter.Done()
}

func optimizePath(
	inPath string,
	colorConversion lossypng.ColorConversion,
	quantization int,
	extension string,
	rewriteOriginal bool,
	addProcessing string,
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

	optimized := lossypng.Compress(decoded, colorConversion, quantization)

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
	var intPercent int64
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
		intPercent = (outSize*100 + inSize/2) / inSize
		percentage = fmt.Sprintf("%d%%", intPercent)
	}

	if intPercent == 0 || intPercent > 99 {
		fmt.Printf("Cannot keep file, compress fail: %d%% \n", intPercent)
		os.Remove(outPath)
		if addProcessing != "" {
			cmdList := strings.Split(addProcessing, " ")
			cmdList = append(cmdList, inPath)
			out, err := exec.Command(cmdList[0], cmdList[1:]...).Output()
			fmt.Printf("Try exec external command: \n %s %s \n ", addProcessing, inPath)
			if err != nil {
				fmt.Printf("Error execution: %s \n ", err)
			} else {
				fmt.Printf("%s\nSuccess\n ", out)
			}
		}

		return
	} else if rewriteOriginal {
		err := os.Rename(outPath, inPath)
		if err == nil {
			outPath = inPath
		} else {
			fmt.Printf("Cannot rewrite original file %s \n", err)
		}
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
