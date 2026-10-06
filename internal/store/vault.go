package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const secretPrefix = "enc:v1:"

func cipherForKey(key []byte) (cipher.AEAD, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(b)
}
func (s *Store) initVault(dir string) error {
	path := filepath.Join(dir, "secrets.key")
	key, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		var count int
		if err = s.db.QueryRow(`select count(*) from accounts where password like 'enc:v1:%' or token like 'enc:v1:%'`).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("凭据密钥 secrets.key 缺失，请恢复原密钥")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, err = f.Write(key)
		if err == nil {
			err = f.Sync()
		}
		f.Close()
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if len(key) != 32 {
		return fmt.Errorf("凭据密钥长度错误")
	}
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	s.vault, err = cipherForKey(key)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`select id,password,token from accounts`)
	if err != nil {
		return err
	}
	type row struct {
		id     int64
		pw, tk string
	}
	var all []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.pw, &r.tk); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range all {
		// Validate existing ciphertext before changing any data.
		pw, e := s.decrypt(r.pw)
		if e != nil {
			return e
		}
		tk, e := s.decrypt(r.tk)
		if e != nil {
			return e
		}
		if strings.HasPrefix(r.pw, secretPrefix) && (r.tk == "" || strings.HasPrefix(r.tk, secretPrefix)) {
			continue
		}
		pw, e = s.encrypt(pw)
		if e != nil {
			return e
		}
		tk, e = s.encrypt(tk)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`update accounts set password=?,token=? where id=?`, pw, tk, r.id); e != nil {
			return e
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = s.db.Exec(`pragma wal_checkpoint(TRUNCATE)`)
	return err
}
func (s *Store) encrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	nonce := make([]byte, s.vault.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	data := s.vault.Seal(nonce, nonce, []byte(value), []byte(secretPrefix))
	return secretPrefix + base64.StdEncoding.EncodeToString(data), nil
}
func (s *Store) decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, secretPrefix) {
		return value, nil
	}
	if s.vault == nil {
		return "", fmt.Errorf("加密凭据缺少密钥")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, secretPrefix))
	if err != nil || len(data) < s.vault.NonceSize() {
		return "", fmt.Errorf("凭据密文损坏")
	}
	n := s.vault.NonceSize()
	raw, err := s.vault.Open(nil, data[:n], data[n:], []byte(secretPrefix))
	if err != nil {
		return "", fmt.Errorf("凭据解密失败，请检查 secrets.key 是否匹配")
	}
	return string(raw), nil
}
