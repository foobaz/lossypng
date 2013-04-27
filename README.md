lossypng
========

lossypng makes PNG files smaller by applying a lossy filter. The compressed PNG
is written to a new file with the extension -lossy.png. With default settings,
lossypng shrinks files by about 50% with minimal loss of quality.

Like JPEG and other lossy image formats, lossypng produces undesirable artifacts
in the image. Image formats that were designed for lossy compression work
better than lossypng but sometimes a PNG is the only option. On the web PNG is
the only format with a full alpha channel. Splash screens for iOS apps must be
PNG.

The algorithm in lossypng works best on direct color images. Indexed color
images with a color palette are compressed using an alternative algorithm.
Passing the -c option will convert an image to 32-bit color and use direct
color compression, which may produce better results. Pass the -g option with
grayscale images to ensure they use direct grayscale instead of a palette.

Images with a lot of flat colors will not compress well with lossypng and may
even increase in size. These images are already well-quantized. Photographic
images compress best.

###Synopsis
`lossypng [options] files...`

###Options
`-c`
Convert image to 32-bit color.

`-e=extension`
Specifies the new filename extension. Defaults to "-lossy.png".

`-g`
Convert image to grayscale.

`-s=strength`
Quantization strength. Defaults to 20. Zero is lossless.

###Installation
`go get github.com/foobaz/lossypng`

###Credit
This compression technique was invented by Michael Vinther for his excellent
Windows program, [Image Analyzer](http://meesoft.logicnet.dk/Analyzer/). It
does much more than just compression. It was ported and improved by William
MacKay.

###Discussion
The main algorithm works by optimizing the image for PNG's average filter. It
quantizes the difference between the pixel and the value predicted by the
average filter. When the image is encoded to PNG, this difference is what gets
sent to zlib for compression. Since most bytes are quantized to a few values
(e.g. -40, -20, 0, 20, 40), zlib is able to store them using less space than if
it had many different numbers.

When the image is decoded, PNG again uses the average filter. The resulting
artifacts are less noticable than traditional quantization with a color palette
because there are fewer sharp transitions between colors.

The compression artifacts produced by the main algorithm show up as dots and
smearing towards the bottom right. Images with a gradient in this direction
exhibit banding. Text remains fully readable, but images with a lot of text will
not compress well.

There is an alternative algorithm for paletted images that optimizes for PNG's
Paeth filter. The average filter does not work well for paletted images because
changing the value of a pixel by one may have a dramatic effect on the color.
The Paeth filter guesses a precise color so it is more suitable. If its guess is
close enough, that color is used. This produces a lot of zeros in the bytes
compressed by zlib. Images compressed with the Paeth filter have different
artifacts which appear as horizontal and vertical banding.

The alternative Paeth algorithm requires an indexed color image and lossypng
cannot convert direct color images to indexed color. To try the Paeth algorithm
quantize these images with another program first, like
[pngnq](http://pngnq.sourceforge.net/) or [pngquant](http://pngquant.org/).
This will improve compression on images with a lot of flat color.

The image files produced by lossypng can be compressed further with advanced
DEFLATE compressors like
[advpng](http://advancemame.sourceforge.net/comp-readme.html) or
[pngout](http://advsys.net/ken/utils.htm).

If multiple input files are given, lossypng will process them in parallel. Most
images will compress in well under a second. Large images may take a few
seconds.

###Improvements
If a PNG file has gamma information it is ignored and discarded. Output images
will therefore render lighter or darker than they should. Gamma information
should be taken into account, or at least preserved, but the PNG codec in Go's
standard library does not offer this functionality.

For some applications, all transparent pixels in the image must remain fully
transparent. An option that leaves all transparent pixels untouched would help.

Currently lossypng compares colors with a simple Euclidian distance between
RGBA values. A perceptual color comparison would work better.

Deep color images with 16-bit samples are converted to 8-bit before processing.
There may be applications where you want a lossy 16-bit image but the
program would have to be updated to work with larger color components.

The holy grail of lossy PNG compression is to go low-level instead and rewrite
zlib to perform lossy comparisons. However, this presents many challenges and
would be a very difficult project.

###License
All code in lossypng is public domain. You may use it however you wish.

###Examples
####Lena, 24-bit direct color
original, 474 kB:

![lena](http://frammish.org/lossypng/lena.png)

-s=20, 89 kB (18% of original):

![lena lossy](http://frammish.org/lossypng/lena-lossy.png)

-s=40, 50 kB (11% of original):

![lena heavy](http://frammish.org/lossypng/lena-heavy.png)

####Tux, 8-bit indexed color with alpha
original, 11.9 kB:

![tux](http://frammish.org/lossypng/Tux.png)

-s=20, 11.4 kB (95% of original):

![tux lossy](http://frammish.org/lossypng/Tux-lossy.png)

-s=40, 8.9 kB (75% of original):

![tux heavy](http://frammish.org/lossypng/Tux-heavy.png)

The Tux image performs poorly because the original image was compressed with
a stronger DEFLATE algorithm than Go's PNG encoder uses. If all three images
are postprocessed with pngout, the original stays the same size, but the two
created by lossypng get even smaller. The image for -s=20 compresses to 9.3 kB
(78%) and -s=40 compresses to 7.6 kB (64%).

####Dice, 32-bit direct color with full alpha
original, 221 kB:

![dice](http://frammish.org/lossypng/dice.png)

-s=20, 75 kB (34% of original):

![dice lossy](http://frammish.org/lossypng/dice-lossy.png)

-s=40, 45 kB (20% of original):

![dice heavy](http://frammish.org/lossypng/dice-heavy.png)
