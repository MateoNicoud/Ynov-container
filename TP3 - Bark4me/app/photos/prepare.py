#!/usr/bin/env -S uv run --script
# /// script
# dependencies = ["pillow"]
# ///
"""Convertit originals/* en images/chien-NN.jpg (800x1000, JPEG q80).

Recadrage centré par défaut ; décalage horizontal par fichier dans CROPX
(0.0 = bord gauche, 0.5 = centre, 1.0 = bord droit).
"""
import sys
from pathlib import Path
from PIL import Image, ImageFilter, ImageOps

HERE = Path(__file__).parent
SRC, DST = HERE / "originals", HERE / "images"
W, H = 800, 1000
CROPX = {"chien-01": 0.45}  # chien + tour Eiffel à droite
# Photos dont le décor compte : image entière (dézoomée) sur fond flou, centrée à FIT[name] de la hauteur
FIT = {"chien-09": 0.42}

DST.mkdir(exist_ok=True)
for old in DST.glob("*.jpg"):
    old.unlink()
files = sorted(p for p in SRC.iterdir() if p.suffix.lower() in {".jpg", ".jpeg", ".png"})
for i, src in enumerate(files, 1):
    name = f"chien-{i:02d}"
    img = ImageOps.exif_transpose(Image.open(src)).convert("RGB")
    if name in FIT:
        # fond : la même image recadrée en 4:5, floutée et assombrie ; premier plan : image entière
        tw0 = round(img.height * W / H)
        bg = img.crop(((img.width - tw0) // 2, 0, (img.width - tw0) // 2 + tw0, img.height)).resize((W, H), Image.LANCZOS)
        bg = bg.filter(ImageFilter.GaussianBlur(28)).point(lambda v: int(v * 0.6))
        fh = round(img.height * W / img.width)
        bg.paste(img.resize((W, fh), Image.LANCZOS), (0, round(H * FIT[name] - fh / 2)))
        bg.save(DST / f"{name}.jpg", "JPEG", quality=80, optimize=True, progressive=True)
        print(f"{src.name} -> {name}.jpg (entière)")
        continue
    cx = CROPX.get(name, 0.5)
    tw = min(img.width, round(img.height * W / H))
    th = round(tw * H / W)
    left = round((img.width - tw) * cx)
    top = round((img.height - th) / 2)
    img = img.crop((left, top, left + tw, top + th)).resize((W, H), Image.LANCZOS)
    img.save(DST / f"{name}.jpg", "JPEG", quality=80, optimize=True, progressive=True)
    print(f"{src.name} -> {name}.jpg")
