package source

import "testing"

// 禁漫乱序段数（对齐站点 reader 的 get_num：key = md5(str(aid)+页码)，页码不含扩展名）
func TestJmSegNum(t *testing.T) {
	cases := []struct {
		name            string
		scrambleID, aid int
		filename        string
		want            int
	}{
		// aid < scramble_id：不乱序，原样保存
		{"below threshold", 220980, 100000, "00001.webp", 0},
		// 220980 <= aid < 268850：固定 10 段
		{"fixed ten", 220980, 220980, "00001.webp", 10},
		{"fixed ten 2", 220980, 268849, "00009.webp", 10},
		// 实测校对过的样本（1238381，用站点 reader 的 scramble 结果逐张验证过分割位置）
		{"1238381 p1", 220980, 1238381, "00001.webp", 16},
		{"1238381 p2", 220980, 1238381, "00002.webp", 8},
		{"1238381 p3", 220980, 1238381, "00003.webp", 4},
		{"1238381 p5", 220980, 1238381, "00005.webp", 12},
		{"1238381 p10", 220980, 1238381, "00010.webp", 2},
		{"1238381 p20", 220980, 1238381, "00020.webp", 16},
		{"1238381 p30", 220980, 1238381, "00030.webp", 6},
		// aid >= 268850 且 < 421926：key % 10
		{"mod ten", 220980, 300000, "00001.webp", 16},
		{"mod ten 2", 220980, 421925, "00007.webp", 8},
		// aid >= 421926：key % 8
		{"mod eight", 220980, 500000, "00003.webp", 14},
	}
	for _, c := range cases {
		if got := jmSegNum(c.scrambleID, c.aid, c.filename); got != c.want {
			t.Errorf("%s: jmSegNum(%d,%d,%q)=%d want %d", c.name, c.scrambleID, c.aid, c.filename, got, c.want)
		}
	}
}
