package settings

// HoursPerDay (quota) and TZOffsetHours (the zone all dates are computed in: ranges, "today",
// worklog days, file timestamps) are read on every call, so configure changes apply without a restart.
func HoursPerDay() float64 { return Number("HOURS_PER_DAY") }

// TZOffsetHours is the UTC offset set in TZ_OFFSET_HOURS; ok is false when it is auto (the device's zone).
func TZOffsetHours() (hours float64, ok bool) {
	if Value("TZ_OFFSET_HOURS") == Auto {
		return 0, false
	}
	return Number("TZ_OFFSET_HOURS"), true
}

// ProjectMap maps a Jira project key to the project code used in the CSV. Unmapped keys are used as-is.
var ProjectMap = map[string]string{
	"CM": "CMOS",
}

// WorkingDaysCountry is the AEXT working-days calendar country.
const WorkingDaysCountry = "RU"

// IgnoredLeaveStatuses do not reduce the quota; any other status except "approved" counts, with a
// warning. Statuses: pending | approved | declined | cancelled.
var IgnoredLeaveStatuses = []string{"declined", "cancelled"}
