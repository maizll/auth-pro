package handler

import "time"

// dayRange 把日历日换成 [当天零点, 次日零点)。
// 查询写成 created_at >= ? AND created_at < ?，避免 DATE(created_at) 让索引失效。
func dayRange(day string) (time.Time, time.Time) {
	start, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	return start, start.AddDate(0, 0, 1)
}
