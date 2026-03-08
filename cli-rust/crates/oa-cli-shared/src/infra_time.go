package infra

import "time"

// timeNowUnixMilli 返回当前时间的 Unix 毫秒时间戳。
// 集中定义以便测试时可替换。
func timeNowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
