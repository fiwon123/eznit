# Icons

`logo.svg` and `logo-horizontal.svg` are the source designs; `logo.png` is the
1024 px raster the build reads (the Windows resource object's icon is generated
from it, and no `.ico` is committed).

The mark is a starting dot and two forward chevrons on a rounded blue square:
"start, quickly" — eznit is *easy init*. The far chevron is a lighter tint so
the direction has depth. It is flat on purpose — no gradient, shadow or badge —
and the shapes are cut as filled polygons, not strokes, so every rasterizer
(the Windows slicer included) draws the same thing at 16 px.

`web/public/` holds the same two files plus a 32 px `favicon.svg`; the web
header draws its own inline copy (`web/src/components/Logo.tsx`) so the
wordmark follows the theme. Keep those copies in step with these.

Regenerate the PNG in the same commit as the SVG it came from:

```sh
magick -background none -density 384 -resize 1024x1024 assets/logo.svg assets/logo.png
```

`internal/tools/winres` slices that PNG into the sizes Explorer asks for
(16/32/48/64/256) and writes the manifest and version block beside them.
