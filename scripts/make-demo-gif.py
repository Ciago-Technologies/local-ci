#!/usr/bin/env python3
"""Render the LocalCI demo as an animated terminal GIF.

Frames show the aha moment: diff finds the missing env var, the
lockfile drift and the Node drift — then `localci run` shows how to
reproduce the failing step. No screen recording needed; the frames
are drawn with PIL.

Usage: python3 scripts/make-demo-gif.py [output.gif]
Requires: Pillow, DejaVu Sans Mono (ships with most Linux distros).
"""
import sys

from PIL import Image, ImageDraw, ImageFont

W, H = 760, 430
BG = (13, 17, 23)       # GitHub dark
FG = (230, 237, 243)
GREEN = (63, 185, 80)
RED = (248, 81, 73)
YELLOW = (210, 153, 34)
MUTED = (139, 148, 158)
ACCENT = (88, 166, 255)

FONT_PATH = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FONT_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"


def font(size, bold=False):
    return ImageFont.truetype(FONT_BOLD if bold else FONT_PATH, size)


# (text, color, bold) lines per frame; each frame adds to the last.
FRAMES = [
    [("$ localci diff local.json ci.json", GREEN, True)],
    [
        ("", FG, False),
        ("Found 7 difference(s) — local vs CI (2 critical, 3 warning, 2 info):", FG, False),
    ],
    [
        ("  [critical] env:STRIPE_KEY: local = set, CI = (missing)", RED, True),
        ("      Fix: Add STRIPE_KEY in repo Settings > Secrets", MUTED, False),
        ("      and variables > Actions, then re-run the workflow.", MUTED, False),
    ],
    [
        ("  [critical] lockfile:package-lock.json: local = a1b2c3, CI = d4e5f6", RED, True),
        ("      Fix: Re-run the install step locally and commit the", MUTED, False),
        ("      updated lockfile.", MUTED, False),
    ],
    [
        ("  [warning] runtime:node: local = 20.11.0, CI = 20.9.0", YELLOW, True),
        ("      Fix: Pin node-version: 20.9.0 in the workflow.", MUTED, False),
    ],
    [
        ("", FG, False),
        ("$ localci run --workflow .github/workflows/ci.yml \\", GREEN, True),
        ("    --job test --step Test", GREEN, True),
        ("# image: ubuntu:24.04 (from runs-on ubuntu-latest)", ACCENT, False),
        ("docker run --rm -v .:/work -w /work ubuntu:24.04 \\", FG, False),
        ("  sh -c 'npm test -- --watchAll=false'", FG, False),
    ],
]


def render(lines):
    img = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(img)
    # Title bar
    d.rectangle([0, 0, W, 34], fill=(22, 27, 34))
    d.ellipse([14, 12, 26, 24], fill=(248, 81, 73))
    d.ellipse([32, 12, 44, 24], fill=(210, 153, 34))
    d.ellipse([50, 12, 62, 24], fill=(63, 185, 80))
    d.text((W // 2 - 40, 9), "localci — demo", font=font(13), fill=MUTED)
    y = 52
    f = font(15)
    for text, color, bold in lines:
        d.text((18, y), text, font=font(15, bold), fill=color)
        y += 24
    # Cursor
    d.rectangle([18, y + 2, 28, y + 20], fill=GREEN)
    return img


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else "docs/demo.gif"
    frames, durations = [], []
    shown = []
    for block in FRAMES:
        shown.extend(block)
        frames.append(render(list(shown)))
        durations.append(1100)
    frames.append(render(list(shown)))
    durations.append(2200)
    frames[0].save(
        out, save_all=True, append_images=frames[1:],
        duration=durations, loop=0, optimize=True,
    )
    print(f"wrote {out} ({len(frames)} frames)")


if __name__ == "__main__":
    main()
