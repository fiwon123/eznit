# Icons

`logo.svg` and `logo-horizontal.svg` are the source designs. `logo.png` is the
1024 px raster the build reads: the Windows resource object's icon is generated
from it, and no `.ico` is committed.

Regenerate the PNG in the same commit as the SVG it came from:

```sh
magick -background none -density 384 -resize 1024x1024 assets/logo.svg assets/logo.png
```

`internal/tools/winres` slices that PNG into the sizes Explorer asks for
(16/32/48/64/256) and writes the manifest and version block beside them.
