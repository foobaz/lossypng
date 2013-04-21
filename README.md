lossypng
========

###Synopsis
lossypng [options] files...

###Description
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

###Options
`-c`
Convert image to 32-bit color.

`-e=extension`
Specifies the new filename extension. Defaults to "-lossy.png".

`-g`
Convert image to grayscale.

`-s=strength`
Quantization strength. Defaults to 16. Zero is lossless.

###Discussion
If multiple input files are given, lossypng will process them in parallel. Most
images will compress in well under a second. Large images may take a few
seconds.

The compression artifacts produced by lossypng show up as dots and smearing
towards the bottom right. Images with a gradient in this direction exhibit
banding. Text remains fully readable, but images with a lot of text will not
compress well.

The main algorithm works by compressing the image using PNG's average filter. It
quantizes the output bytes of the PNG file, but since the image is decoded
using the average filter, the pixel color values are not quantized and have full
range. The resulting artifacts are less noticable than traditional quantization
in color space.

There is an alternative algorithm for paletted images that optimizes for PNG's
Paeth filter. The average filter does not work well for paletted images because
changing the value of a pixel by one may have a dramatic effect on the color.
The Paeth filter guesses a precise color so it is more suitable. Images
compressed with the Paeth filter have different artifacts which appear as
horizontal and vertical banding.

The alternative Paeth algorithm requires an indexed color image and lossypng
cannot convert direct color images to indexed color. To try the Paeth algorithm
quantize these images with another program first, like pngnq or pngquant. This
will improve compression on images with a lot of flat color.

The image files produced by lossypng can be compressed further with advanced
DEFLATE compressors like advpng or pngout.

###Improvements
For some applications, all transparent pixels in the image must remain fully
transparent. An option that leaves all transparent pixels untouched would help.

The quantization algorithm could be changed to consider an area of pixels
instead of just the neighbors. It could use this information to dither and
prevent banding.

Currently lossypng compares colors with a simple Euclidian distance between
RGBA values. A perceptual color comparison would work better.

Deep color images with 16-bit samples are converted to 8-bit before processing.
There may be applications where you want a lossy 16-bit image but the
program would have to be updated to work with larger color components.

The holy grail of lossy PNG compression is to go low-level instead and rewrite
zlib to perform lossy comparisons. However, this presents many challenges and
would be a very difficult project.

###Examples
Lena, original, 463kB:

![lena](http://frammish.org/lossypng/lena.png)

Lena, -s=16, 142kB:

![lena lossy](http://frammish.org/lossypng/lena-lossy.png)

Lena, -s=40, 65kB:

![lena heavy](http://frammish.org/lossypng/lena-heavy.png)

Tux, original, 12kB:

![tux](http://frammish.org/lossypng/Tux.png)

Tux, -s=16, 13kB (larger!):

![tux lossy](http://frammish.org/lossypng/Tux-lossy.png)

Tux, -s=40, 8kB:

![tux heavy](http://frammish.org/lossypng/Tux-heavy.png)
