package api

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

// 阅读器缓存（cache/reader/<kind>/<comicID>/<order>/）是回源结果的落盘副本，
// 只为「翻页/重看不重复走乱序还原」而存在，属于纯派生数据：
// 无限增长会慢慢吃掉 NAS 的空间，删掉最坏也只是多一次回源，所以按访问时间定期清理。

// lastAccess 文件的最近访问时间：Linux 上取 atime/mtime 的较大者（只读不改的缓存
// 用 atime 才能反映「最近还在看」），取不到 atime 就退回 mtime
func lastAccess(fi os.FileInfo) time.Time {
	t := fi.ModTime()
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st != nil {
		if a := time.Unix(st.Atim.Sec, st.Atim.Nsec); a.After(t) {
			t = a
		}
	}
	return t
}

// CleanReaderCache 删除 maxAge 未被访问过的缓存页文件，并顺手清掉空目录。
// 返回 (删除文件数, 释放字节数)。
func (s *Server) CleanReaderCache(maxAge time.Duration) (removed int, freed int64) {
	root := filepath.Join(s.base, "reader")
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return 0, 0
	}
	cutoff := time.Now().Add(-maxAge)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		fi, ierr := d.Info()
		if ierr != nil || lastAccess(fi).After(cutoff) {
			return nil
		}
		if rerr := os.Remove(p); rerr == nil {
			removed++
			freed += fi.Size()
		}
		return nil
	})
	pruneEmptyDirs(root)
	return removed, freed
}

// pruneEmptyDirs 自底向上删空目录（非空的删失败即忽略）
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs))) // 路径深的先删
	for _, d := range dirs {
		_ = os.Remove(d)
	}
}
