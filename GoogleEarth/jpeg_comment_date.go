package GoogleEarth

import (
	"fmt"
	"time"
)

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

// JpegCommentDate JPEG 注释日期
// 用于表示 Google Earth 历史影像的日期信息
type JpegCommentDate struct {
	year  int16 // 年份（0 表示未知）
	month int8  // 月份（1-12，0 表示未知）
	day   int8  // 日期（1-31，0 表示未知）
}

// NewJpegCommentDate 创建 JPEG 注释日期
func NewJpegCommentDate(year int16, month, day int8) JpegCommentDate {
	return JpegCommentDate{
		year:  year,
		month: month,
		day:   day,
	}
}

// NewJpegCommentDateFromInt 从整数创建日期
// 格式：YYYYMMDD（如 20231115 表示 2023年11月15日）
func NewJpegCommentDateFromInt(dateInt int32) JpegCommentDate {
	if dateInt == 0 {
		return JpegCommentDate{}
	}

	year := int16(dateInt / 10000)
	month := int8((dateInt % 10000) / 100)
	day := int8(dateInt % 100)

	return JpegCommentDate{
		year:  year,
		month: month,
		day:   day,
	}
}

// NewJpegCommentDateFromTime 从 time.Time 创建日期
func NewJpegCommentDateFromTime(t time.Time) JpegCommentDate {
	return JpegCommentDate{
		year:  int16(t.Year()),
		month: int8(t.Month()),
		day:   int8(t.Day()),
	}
}

// Year 获取年份
func (d JpegCommentDate) Year() int16 {
	return d.year
}

// Month 获取月份
func (d JpegCommentDate) Month() int8 {
	return d.month
}

// Day 获取日期
func (d JpegCommentDate) Day() int8 {
	return d.day
}

// IsCompletelyUnknown 判断日期是否完全未知
func (d JpegCommentDate) IsCompletelyUnknown() bool {
	return d.year == 0 && d.month == 0 && d.day == 0
}

// IsYearKnown 判断年份是否已知
func (d JpegCommentDate) IsYearKnown() bool {
	return d.year != 0
}

// IsMonthKnown 判断月份是否已知
func (d JpegCommentDate) IsMonthKnown() bool {
	return d.month != 0
}

// IsDayKnown 判断日期是否已知
func (d JpegCommentDate) IsDayKnown() bool {
	return d.day != 0
}

// MatchAllDates 判断是否匹配所有日期
// 用于特殊标记，表示需要获取所有历史影像
func (d JpegCommentDate) MatchAllDates() bool {
	return d.year == -1 && d.month == -1 && d.day == -1
}

// ToInt 转换为整数格式
func (d JpegCommentDate) ToInt() int32 {
	if d.IsCompletelyUnknown() {
		return 0
	}
	return int32(d.year)*10000 + int32(d.month)*100 + int32(d.day)
}

// ToString 转换为字符串格式
func (d JpegCommentDate) ToString() string {
	if d.IsCompletelyUnknown() {
		return "Unknown"
	}

	if d.MatchAllDates() {
		return "MatchAll"
	}

	if !d.IsDayKnown() {
		if !d.IsMonthKnown() {
			return fmt.Sprintf("%04d", d.year)
		}
		return fmt.Sprintf("%04d-%02d", d.year, d.month)
	}

	return fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day)
}

// ToTime 转换为 time.Time
// 如果日期不完整，使用默认值填充
func (d JpegCommentDate) ToTime() (time.Time, error) {
	if d.IsCompletelyUnknown() {
		return time.Time{}, fmt.Errorf("date is completely unknown")
	}

	year := int(d.year)
	month := time.Month(d.month)
	day := int(d.day)

	if month == 0 {
		month = time.January
	}
	if day == 0 {
		day = 1
	}

	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

// CompareTo 比较两个日期
// 返回：-1 表示小于，0 表示相等，1 表示大于
func (d JpegCommentDate) CompareTo(other JpegCommentDate) int {
	if d.year != other.year {
		if d.year < other.year {
			return -1
		}
		return 1
	}

	if d.month != other.month {
		if d.month < other.month {
			return -1
		}
		return 1
	}

	if d.day != other.day {
		if d.day < other.day {
			return -1
		}
		return 1
	}

	return 0
}

// Equal 判断两个日期是否相等
func (d JpegCommentDate) Equal(other JpegCommentDate) bool {
	return d.year == other.year && d.month == other.month && d.day == other.day
}

// Before 判断是否在另一个日期之前
func (d JpegCommentDate) Before(other JpegCommentDate) bool {
	return d.CompareTo(other) < 0
}

// After 判断是否在另一个日期之后
func (d JpegCommentDate) After(other JpegCommentDate) bool {
	return d.CompareTo(other) > 0
}

// ParseJpegCommentDateString 从字符串解析日期
// 支持格式：YYYY-MM-DD, YYYY-MM, YYYY, YYYYMMDD
func ParseJpegCommentDateString(s string) (JpegCommentDate, error) {
	if s == "" || s == "Unknown" {
		return JpegCommentDate{}, nil
	}

	if s == "MatchAll" {
		return JpegCommentDate{year: -1, month: -1, day: -1}, nil
	}

	// 尝试解析 YYYYMMDD 格式
	if len(s) == 8 {
		var dateInt int32
		_, err := fmt.Sscanf(s, "%d", &dateInt)
		if err == nil {
			return NewJpegCommentDateFromInt(dateInt), nil
		}
	}

	// 尝试解析 YYYY-MM-DD 格式
	var year int16
	var month, day int8

	n, err := fmt.Sscanf(s, "%d-%d-%d", &year, &month, &day)
	if err == nil && n == 3 {
		return JpegCommentDate{year: year, month: month, day: day}, nil
	}

	// 尝试解析 YYYY-MM 格式
	n, err = fmt.Sscanf(s, "%d-%d", &year, &month)
	if err == nil && n == 2 {
		return JpegCommentDate{year: year, month: month, day: 0}, nil
	}

	// 尝试解析 YYYY 格式
	n, err = fmt.Sscanf(s, "%d", &year)
	if err == nil && n == 1 {
		return JpegCommentDate{year: year, month: 0, day: 0}, nil
	}

	return JpegCommentDate{}, fmt.Errorf("invalid date format: %s", s)
}
