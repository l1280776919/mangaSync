package source

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // 注册解码器
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	_ "golang.org/x/image/webp" // 注册 webp 解码器
)

// 禁漫的图片乱序（scramble）常量，对齐 jmcomic 的 JmMagicConstants
const (
	jmScramble220980 = 220980
	jmScramble268850 = 268850
	jmScramble421926 = 421926
)

// jmSegNum 计算某张图被切成几段，0 表示没乱序、可原样保存。
// 对齐 JmImageTool.get_num：先看本子 id 与 scramble_id 的关系，再按 md5(aid+文件名) 定段数。
func jmSegNum(scrambleID, aid int, filename string) int {
	if aid < scrambleID {
		return 0
	}
	if aid < jmScramble268850 {
		return 10
	}
	x := 8
	if aid < jmScramble421926 {
		x = 10
	}
	sum := md5Hex(fmt.Sprintf("%d%s", aid, filename))
	return int(sum[len(sum)-1])%x*2 + 2
}

// jmToRGBA 把解码结果转成 RGBA。
//
// 注意：Go 标准库的 image.YCbCr→RGBA 用的是 JFIF 全范围矩阵，而 VP8（webp 有损）存的是
// 有限范围 YUV，直接 draw 会整体偏暗十几个色阶（实测均值 187 vs libwebp 199）。
// 这里对 YCbCr 自己做 BT.601 有限范围转换（整数定点，与 libwebp/浏览器观感一致）；
// 其它格式（PNG/GIF/JPEG）交给 draw 即可。
func jmToRGBA(src image.Image) *image.RGBA {
	if ycc, ok := src.(*image.YCbCr); ok {
		b := ycc.Bounds()
		dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				yy := int(ycc.Y[ycc.YOffset(x, y)]) - 16
				cb := int(ycc.Cb[ycc.COffset(x, y)]) - 128
				cr := int(ycc.Cr[ycc.COffset(x, y)]) - 128
				i := dst.PixOffset(x-b.Min.X, y-b.Min.Y)
				dst.Pix[i] = clip8((298*yy + 409*cr + 128) >> 8)
				dst.Pix[i+1] = clip8((298*yy - 100*cb - 208*cr + 128) >> 8)
				dst.Pix[i+2] = clip8((298*yy + 516*cb + 128) >> 8)
				dst.Pix[i+3] = 255
			}
		}
		return dst
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func clip8(v int) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v)
}

// jmDescramble 把按行带乱序的图还原：从下往上取 num 段，自上而下贴回。
// 对齐 JmImageTool.decode_and_save（注意每轮 move 都要重置，且首段额外带上 h%num 的余数）。
func jmDescramble(src image.Image, num int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	srcRGBA := jmToRGBA(src)
	if num <= 0 {
		draw.Draw(dst, dst.Bounds(), srcRGBA, image.Point{}, draw.Src)
		return dst
	}
	over := h % num
	for i := 0; i < num; i++ {
		move := h / num
		ySrc := h - move*(i+1) - over
		yDst := move * i
		if i == 0 {
			move += over
		} else {
			yDst += over
		}
		if move <= 0 || ySrc < 0 || yDst < 0 || ySrc+move > h || yDst+move > h {
			continue // 尺寸异常时跳过该段，避免越界
		}
		draw.Draw(dst,
			image.Rect(0, yDst, w, yDst+move),
			srcRGBA, image.Pt(0, ySrc), draw.Src)
	}
	return dst
}

// jmSaveImage 保存一张禁漫图片：
//   - num == 0：没有乱序，直接落原始字节（与站点下发的完全一致，零损失）
//   - num > 0 ：必须重排，解码后无损编码成 WebP（nativewebp 走 VP8L，不引入二次损失）；
//     万一编码失败则回落 PNG（同样无损，只是体积大些）
//
// 返回实际写入的路径（回落 PNG 时扩展名会变）。
func jmSaveImage(raw []byte, num int, dstPath string) (string, error) {
	if num <= 0 {
		if err := writeFileAtomic(dstPath, raw); err != nil {
			return "", err
		}
		return dstPath, nil
	}
	data, ext, err := jmEncodePage(raw, num, "lossless")
	if err != nil {
		return "", err
	}
	if ext == "png" {
		dstPath = strings.TrimSuffix(dstPath, filepath.Ext(dstPath)) + ".png"
	}
	if err := writeFileAtomic(dstPath, data); err != nil {
		return "", err
	}
	return dstPath, nil
}

// jmEncodePage 解码 + 还原乱序 + 编码，返回图片字节与扩展名（不落盘）。
// mode:
//   - "lossless"：无损 WebP（下载落盘用，零二次损失，体积大）
//   - "jpeg"    ：高质量 JPEG（在线阅读用，体积约为无损的 1/4，肉眼看不出差别）
//
// 若原图本来没有乱序，两种模式都直接原样返回（零损失、零重编码）。
func jmEncodePage(raw []byte, num int, mode string) ([]byte, string, error) {
	if num <= 0 {
		return raw, "webp", nil
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("解码失败: %w", err)
	}
	fixed := jmDescramble(src, num)

	if mode == "jpeg" {
		var jb bytes.Buffer
		if err := jpeg.Encode(&jb, fixed, &jpeg.Options{Quality: 88}); err != nil {
			return nil, "", fmt.Errorf("JPEG 编码失败: %w", err)
		}
		return jb.Bytes(), "jpg", nil
	}

	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, fixed, nil); err == nil {
		if out, _, derr := image.Decode(bytes.NewReader(buf.Bytes())); derr == nil && out.Bounds() == fixed.Bounds() {
			return buf.Bytes(), "webp", nil
		}
	}

	// 回落：无损 PNG
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, fixed); err != nil {
		return nil, "", fmt.Errorf("编码失败: %w", err)
	}
	return pngBuf.Bytes(), "png", nil
}

// writeFileAtomic 先写临时文件再改名，避免中断留下半张图
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
