package quota

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"modelgate/internal/entity"
)

func makeTime(hour, min int) time.Time {
	return time.Date(2026, 3, 19, hour, min, 0, 0, time.Local)
}

func TestIsWithinAvailableTime_EmptyRanges(t *testing.T) {
	// 空列表 = 全天可用
	assert.True(t, isWithinAvailableTime(nil, makeTime(12, 0)))
	assert.True(t, isWithinAvailableTime([]entity.TimeRange{}, makeTime(12, 0)))
}

func TestIsWithinAvailableTime_SingleRange(t *testing.T) {
	ranges := []entity.TimeRange{{Start: "08:00", End: "18:00"}}

	assert.True(t, isWithinAvailableTime(ranges, makeTime(8, 0)))   // 边界：开始时间
	assert.True(t, isWithinAvailableTime(ranges, makeTime(12, 0)))  // 中间
	assert.True(t, isWithinAvailableTime(ranges, makeTime(17, 59))) // 接近结束
	assert.False(t, isWithinAvailableTime(ranges, makeTime(18, 0))) // 边界：结束时间（不含）
	assert.False(t, isWithinAvailableTime(ranges, makeTime(7, 59))) // 之前
	assert.False(t, isWithinAvailableTime(ranges, makeTime(23, 0))) // 之后
}

func TestIsWithinAvailableTime_MultipleRanges(t *testing.T) {
	ranges := []entity.TimeRange{
		{Start: "00:00", End: "10:00"},
		{Start: "18:00", End: "24:00"},
	}

	assert.True(t, isWithinAvailableTime(ranges, makeTime(0, 0)))   // 凌晨
	assert.True(t, isWithinAvailableTime(ranges, makeTime(9, 59)))  // 第一段末尾
	assert.False(t, isWithinAvailableTime(ranges, makeTime(10, 0))) // 间隔
	assert.False(t, isWithinAvailableTime(ranges, makeTime(15, 0))) // 间隔
	assert.True(t, isWithinAvailableTime(ranges, makeTime(18, 0)))  // 第二段开始
	assert.True(t, isWithinAvailableTime(ranges, makeTime(23, 59))) // 第二段末尾
}

func TestIsWithinAvailableTime_CrossMidnight(t *testing.T) {
	ranges := []entity.TimeRange{{Start: "22:00", End: "06:00"}}

	assert.True(t, isWithinAvailableTime(ranges, makeTime(22, 0)))  // 开始
	assert.True(t, isWithinAvailableTime(ranges, makeTime(23, 30))) // 晚上
	assert.True(t, isWithinAvailableTime(ranges, makeTime(0, 0)))   // 午夜
	assert.True(t, isWithinAvailableTime(ranges, makeTime(3, 0)))   // 凌晨
	assert.True(t, isWithinAvailableTime(ranges, makeTime(5, 59)))  // 接近结束
	assert.False(t, isWithinAvailableTime(ranges, makeTime(6, 0)))  // 结束时间
	assert.False(t, isWithinAvailableTime(ranges, makeTime(12, 0))) // 白天
	assert.False(t, isWithinAvailableTime(ranges, makeTime(21, 59))) // 开始前
}

func TestIsWithinAvailableTime_FullDay(t *testing.T) {
	ranges := []entity.TimeRange{{Start: "00:00", End: "24:00"}}

	assert.True(t, isWithinAvailableTime(ranges, makeTime(0, 0)))
	assert.True(t, isWithinAvailableTime(ranges, makeTime(12, 0)))
	assert.True(t, isWithinAvailableTime(ranges, makeTime(23, 59)))
}

func TestIsWithinAvailableTime_InvalidFormat(t *testing.T) {
	// 无效格式应被忽略（等同于空列表 → 但因为至少有条目，不匹配则返回 false）
	ranges := []entity.TimeRange{{Start: "invalid", End: "also-invalid"}}
	assert.False(t, isWithinAvailableTime(ranges, makeTime(12, 0)))
}

func TestParseTimeOfDay(t *testing.T) {
	tests := []struct {
		input string
		h, m  int
		err   bool
	}{
		{"00:00", 0, 0, false},
		{"08:30", 8, 30, false},
		{"12:00", 12, 0, false},
		{"23:59", 23, 59, false},
		{"24:00", 24, 0, false},
		{"24:01", 0, 0, true},
		{"25:00", 0, 0, true},
		{"ab:cd", 0, 0, true},
		{"8:00", 0, 0, true},   // 不足5位
		{"", 0, 0, true},
	}

	for _, tt := range tests {
		h, m, err := parseTimeOfDay(tt.input)
		if tt.err {
			assert.Error(t, err, "expected error for %q", tt.input)
		} else {
			assert.NoError(t, err, "unexpected error for %q", tt.input)
			assert.Equal(t, tt.h, h, "hour mismatch for %q", tt.input)
			assert.Equal(t, tt.m, m, "minute mismatch for %q", tt.input)
		}
	}
}
