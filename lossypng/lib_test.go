package lossypng

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"
)

func TestCompression(t *testing.T) {
	for _, name := range [...]string{"sample.png", "paletted.png"} {
		file, err := os.Open(name)
		if err != nil {
			t.Fatalf("couldn't open file %s", name)
		}
		info, err := file.Stat()
		if err != nil {
			t.Fatalf("couldn't read file size of %s", name)
		}
		originalSize := info.Size()
		defer file.Close()
		img, _, err := image.Decode(file)
		if err != nil {
			t.Fatalf("couldn't decode file %s", name)
		}
		modes := [...]ColorConversion{
			NoConversion,
			GrayscaleConversion,
			RGBAConversion,
		}
		for _, mode := range modes {
			compressed := Compress(img, mode, 20)
			buf := new(bytes.Buffer)
			err := png.Encode(buf, compressed)
			if err != nil {
				t.Fatalf("couldn't encode file %d", name)
			}
			if int64(buf.Len()) >= originalSize {
				t.Fatalf("sample %s did not compress in mode %d", name, mode)
			}
		}
	}
}
