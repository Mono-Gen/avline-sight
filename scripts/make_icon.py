"""AVLine-Sight のアイコン（ICO）を依存なしで生成する。

青緑の丸に白い脈波線（監視の意味）。各サイズを4倍で描いて縮小し、縁を滑らかにする。
ICO の各画像は PNG 形式（Windows Vista 以降で読める）。
使い方: python scripts/make_icon.py <出力.ico> [プレビュー.png]
"""
import math
import struct
import sys
import zlib

SIZES = [16, 20, 24, 32, 40, 48, 64, 128, 256]
TEAL_TOP = (0x1f, 0xc9, 0x9a)
TEAL_BOTTOM = (0x0f, 0x8f, 0x6c)
# 脈波線の折れ点（0〜1の座標、上が0）
PULSE = [(0.18, 0.55), (0.38, 0.55), (0.47, 0.30), (0.57, 0.74), (0.65, 0.48), (0.82, 0.48)]


def seg_dist(px, py, ax, ay, bx, by):
    dx, dy = bx - ax, by - ay
    t = max(0.0, min(1.0, ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy)))
    return math.hypot(px - (ax + t * dx), py - (ay + t * dy))


def render(n):
    ss = 4
    big = n * ss
    # 小さいサイズほど線を太くして潰れないようにする
    width = 0.11 if n <= 20 else 0.085 if n <= 48 else 0.07
    rows = []
    for y in range(n):
        row = []
        for x in range(n):
            acc = [0.0, 0.0, 0.0, 0.0]
            for sy in range(ss):
                for sx in range(ss):
                    u = (x * ss + sx + 0.5) / big
                    v = (y * ss + sy + 0.5) / big
                    d = math.hypot(u - 0.5, v - 0.5)
                    if d > 0.48:
                        continue
                    k = v
                    col = [TEAL_TOP[i] * (1 - k) + TEAL_BOTTOM[i] * k for i in range(3)]
                    line = min(seg_dist(u, v, *PULSE[i], *PULSE[i + 1]) for i in range(len(PULSE) - 1))
                    if line < width / 2:
                        col = [255, 255, 255]
                    for i in range(3):
                        acc[i] += col[i]
                    acc[3] += 1
            a = acc[3] / (ss * ss)
            if acc[3]:
                row.append((round(acc[0] / acc[3]), round(acc[1] / acc[3]), round(acc[2] / acc[3]), round(a * 255)))
            else:
                row.append((0, 0, 0, 0))
        rows.append(row)
    return rows


def png(rows):
    n = len(rows)
    raw = b"".join(b"\x00" + bytes(c for px in row for c in px) for row in rows)

    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)

    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", n, n, 8, 6, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def main():
    out = sys.argv[1]
    images = [(n, png(render(n))) for n in SIZES]
    head = struct.pack("<HHH", 0, 1, len(images))
    offset = 6 + 16 * len(images)
    body = b""
    for n, data in images:
        side = 0 if n == 256 else n
        head += struct.pack("<BBBBHHII", side, side, 0, 0, 1, 32, len(data), offset + len(body))
        body += data
    with open(out, "wb") as f:
        f.write(head + body)
    if len(sys.argv) > 2:
        with open(sys.argv[2], "wb") as f:
            f.write(dict(images)[256])


if __name__ == "__main__":
    main()
