package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronField represents a parsed cron field.
type CronField struct {
	values []int
}

// CronSchedule represents a parsed 5-field cron expression.
type CronSchedule struct {
	Minute  CronField
	Hour    CronField
	Day     CronField
	Month   CronField
	Weekday CronField
}

var (
	allMinutes  = makeRange(0, 59)
	allHours    = makeRange(0, 23)
	allDays     = makeRange(1, 31)
	allMonths   = makeRange(1, 12)
	allWeekdays = makeRange(0, 6)
)

func makeRange(min, max int) []int {
	vals := make([]int, max-min+1)
	for i := range vals {
		vals[i] = min + i
	}
	return vals
}

// ParseCron parses a 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week
// Supports: * (wildcard), numbers, ranges (1-5), lists (1,3,5), step values (*/5, 1-10/2)
func ParseCron(expr string) (*CronSchedule, error) {
	expr = strings.TrimSpace(expr)
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d | hint: use format 'minute hour day month weekday', e.g. '*/5 * * * *' for every 5 minutes", len(fields))
	}

	minute, err := parseField(fields[0], 0, 59, "minute")
	if err != nil {
		return nil, err
	}
	hour, err := parseField(fields[1], 0, 23, "hour")
	if err != nil {
		return nil, err
	}
	day, err := parseField(fields[2], 1, 31, "day")
	if err != nil {
		return nil, err
	}
	month, err := parseField(fields[3], 1, 12, "month")
	if err != nil {
		return nil, err
	}
	weekday, err := parseField(fields[4], 0, 6, "weekday")
	if err != nil {
		return nil, err
	}

	return &CronSchedule{
		Minute:  CronField{values: minute},
		Hour:    CronField{values: hour},
		Day:     CronField{values: day},
		Month:   CronField{values: month},
		Weekday: CronField{values: weekday},
	}, nil
}

func parseField(field string, min, max int, name string) ([]int, error) {
	if field == "*" {
		return makeRange(min, max), nil
	}

	var result []int
	parts := strings.Split(field, ",")

	for _, part := range parts {
		step := 1
		stepIdx := strings.Index(part, "/")
		if stepIdx >= 0 {
			s, err := strconv.Atoi(part[stepIdx+1:])
			if err != nil || s <= 0 {
				return nil, fmt.Errorf("invalid step in %s field: %s | hint: step must be a positive integer", name, part)
			}
			step = s
			part = part[:stepIdx]
		}

		var lo, hi int
		if part == "*" {
			lo, hi = min, max
		} else if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			var err error
			lo, err = strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start in %s field: %s", name, part)
			}
			hi, err = strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end in %s field: %s", name, part)
			}
		} else {
			val, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value in %s field: %s | hint: use numbers, ranges (1-5), lists (1,3,5), or wildcards (*/5)", name, part)
			}
			lo, hi = val, val
		}

		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("value out of range in %s field: %s (valid: %d-%d)", name, part, min, max)
		}

		for v := lo; v <= hi; v += step {
			result = append(result, v)
		}
	}

	return result, nil
}

func (f CronField) contains(val int) bool {
	for _, v := range f.values {
		if v == val {
			return true
		}
	}
	return false
}

// Next calculates the next time after `from` that matches the cron schedule.
func (c *CronSchedule) Next(from time.Time) time.Time {
	// Start from the next minute
	t := from.Truncate(time.Minute).Add(time.Minute)

	for i := 0; i < 366*24*60; i++ { // max 1 year
		if c.Month.contains(int(t.Month())) &&
			c.Day.contains(t.Day()) &&
			c.Weekday.contains(int(t.Weekday())) &&
			c.Hour.contains(t.Hour()) &&
			c.Minute.contains(t.Minute()) {
			return t
		}
		t = t.Add(time.Minute)
	}

	return time.Time{} // no match found within a year
}

// ValidateSchedule validates a schedule string and returns the schedule type.
func ValidateSchedule(schedule string) (string, error) {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return "", fmt.Errorf("schedule is required | hint: use a cron expression like '*/5 * * * *' or an interval like '5m', '1h'")
	}

	// Try interval first
	if _, err := ParseInterval(schedule); err == nil {
		return "interval", nil
	}

	// Try cron
	if _, err := ParseCron(schedule); err == nil {
		return "cron", nil
	}

	return "", fmt.Errorf("invalid schedule: %s | hint: use a cron expression like '*/5 * * * *' or an interval like '5m', '1h', '30s'", schedule)
}

// CalculateNextRun computes the next run time for a job.
func CalculateNextRun(schedule, scheduleType string, from time.Time) (time.Time, error) {
	if scheduleType == "interval" {
		d, err := ParseInterval(schedule)
		if err != nil {
			return time.Time{}, err
		}
		return from.Add(d), nil
	}

	c, err := ParseCron(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return c.Next(from), nil
}
