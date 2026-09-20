// Package auth 提供口令派生、会话令牌与登录限流等基础能力（全部仅用标准库）。
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Iterations PBKDF2 迭代次数（登录一次约几十毫秒）
const Iterations = 200000

// MinPasswordLen 最短口令长度
const MinPasswordLen = 6

// ErrWeakPassword 口令太短
var ErrWeakPassword = errors.New("新密码至少 6 位")

// HashPassword 生成 base64(盐) 与 base64(派生值)
func HashPassword(password string) (hashB64, saltB64 string, iterations int, err error) {
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return "", "", 0, err
	}
	dk := pbkdf2SHA256([]byte(password), salt, Iterations, 32)
	return base64.StdEncoding.EncodeToString(dk), base64.StdEncoding.EncodeToString(salt), Iterations, nil
}

// VerifyPassword 校验口令（常量时间比较）
func VerifyPassword(password, hashB64, saltB64 string, iterations int) bool {
	if iterations <= 0 {
		iterations = Iterations
	}
	salt, err1 := base64.StdEncoding.DecodeString(saltB64)
	want, err2 := base64.StdEncoding.DecodeString(hashB64)
	if err1 != nil || err2 != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// CheckPassword 只做长度等基础校验
func CheckPassword(pw string) error {
	if len([]rune(pw)) < MinPasswordLen {
		return ErrWeakPassword
	}
	return nil
}

// NewToken 生成 32 字节随机会话令牌（hex）
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// pbkdf2SHA256 标准 PBKDF2-HMAC-SHA256 实现（等价于 x/crypto/pbkdf2）
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		_, _ = prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		_, _ = prf.Write(buf[:4])
		dk = prf.Sum(dk)
		t := dk[len(dk)-hashLen:]
		copy(u, t)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			_, _ = prf.Write(u)
			u = u[:0]
			u = prf.Sum(u)
			for x := range u {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

// ---------------------------- 登录限流 ----------------------------

// limiterEntry 某个 key（用户名）的失败记录
type limiterEntry struct {
	fails     int
	firstFail time.Time
	lockedTo  time.Time
}

// Limiter 简易登录限流：同账号连续失败 MaxFails 次后锁定 LockFor。
// 说明：穿透/反代场景下客户端 IP 都是 127.0.0.1，所以按用户名计数。
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry

	MaxFails int
	Window   time.Duration
	LockFor  time.Duration
}

// NewLimiter 默认：10 分钟内失败 5 次 → 锁 60 秒
func NewLimiter() *Limiter {
	return &Limiter{entries: map[string]*limiterEntry{}, MaxFails: 5, Window: 10 * time.Minute, LockFor: 60 * time.Second}
}

// Allow 是否允许尝试；被锁时返回剩余秒数
func (l *Limiter) Allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	now := time.Now()
	if now.Before(e.lockedTo) {
		return false, int(e.lockedTo.Sub(now).Seconds()) + 1
	}
	if now.Sub(e.firstFail) > l.Window {
		delete(l.entries, key)
	}
	return true, 0
}

// Fail 记一次失败
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e, ok := l.entries[key]
	if !ok || now.Sub(e.firstFail) > l.Window {
		// 控制内存：条目过多时整体清理过窗口的
		if len(l.entries) > 512 {
			for k, v := range l.entries {
				if now.Sub(v.firstFail) > l.Window {
					delete(l.entries, k)
				}
			}
		}
		l.entries[key] = &limiterEntry{fails: 1, firstFail: now}
		return
	}
	e.fails++
	if e.fails >= l.MaxFails {
		e.lockedTo = now.Add(l.LockFor)
		e.fails = 0
		e.firstFail = now
	}
}

// Reset 登录成功后清零
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}
