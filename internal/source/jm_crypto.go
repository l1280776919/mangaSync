package source

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// 禁漫移动端接口的加密规则（对齐 jmcomic 的 JmCryptoTool）：
//
//	请求头 token  = md5(时间戳 + 密钥)    （密钥随接口不同，图片/章节模板另有专用密钥）
//	请求头 tokenparam = "时间戳,版本号"
//	响应体 data   = base64(AES-ECB(md5(时间戳+密钥), 明文JSON))  —— 解密后去掉 PKCS#7 填充
const (
	jmTokenSecret    = "185Hcomic3PAPP7R"  // 常规接口密钥
	jmDataSecret     = "185Hcomic3PAPP7R"  // 响应解密密钥
	jmScrambleSecret = "18comicAPPContent" // /chapter_view_template 专用（用错会 403）
	jmAppVersion     = "2.1.8"

	jmMobileUA = "Mozilla/5.0 (Linux; Android 9; V1938CT Build/PQ3A.190705.11211812; wv) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/91.0.4472.114 Safari/537.36"
)

// 禁漫域名（移动端 API 与图片 CDN），默认列表；实际用 /setting 返回的列表覆盖
var (
	jmAPIDomains   = []string{"www.cdngwc.club", "www.cdngwc.net", "www.cdngwc.cc", "www.cdnhjk.net"}
	jmImageDomains = []string{
		"cdn-msp.jmapinodeudzn.net", "cdn-msp3.jmapinodeudzn.net", "cdn-msp2.jmapiproxy2.cc",
		"cdn-msp.jmapiproxy2.cc", "cdn-msp3.jmapiproxy2.cc", "cdn-msp.jmapiproxy1.cc",
	}
)

var errJMBadCipher = errors.New("禁漫响应解密失败")

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// jmToken 计算请求头 token
func jmToken(ts int64, secret string) string {
	return md5Hex(fmt.Sprintf("%d%s", ts, secret))
}

// jmDecodeData 解密响应里的 data 字段（base64 → AES-ECB → 去填充）
func jmDecodeData(b64 string, ts int64, secret string) ([]byte, error) {
	cipherText, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 解码失败: %v", errJMBadCipher, err)
	}
	key := []byte(md5Hex(fmt.Sprintf("%d%s", ts, secret)))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errJMBadCipher, err)
	}
	if len(cipherText) == 0 || len(cipherText)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("%w: 密文长度 %d 非法", errJMBadCipher, len(cipherText))
	}
	plain := make([]byte, len(cipherText))
	for i := 0; i < len(cipherText); i += aes.BlockSize {
		block.Decrypt(plain[i:i+aes.BlockSize], cipherText[i:i+aes.BlockSize])
	}
	// 去 PKCS#7 填充
	pad := int(plain[len(plain)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(plain) {
		return nil, fmt.Errorf("%w: 填充字节 %d 非法", errJMBadCipher, pad)
	}
	return plain[:len(plain)-pad], nil
}
