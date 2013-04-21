lossypng
========

lossypng makes smaller PNG files by applying a lossy filter. With default
settings, it shrinks files by an average of 50% with minimal loss of quality.


Switches
Normal/Average
- artifacting, smears towards bottom right
Paletted/Paeth
- not as good, artifacting is blocky
- suggest -c
- suggest pngnq/pngquant
Improvements
- zlib
- perceptual/psycho
- transparency
- 16-bit color
- quantize to palette
- dither
