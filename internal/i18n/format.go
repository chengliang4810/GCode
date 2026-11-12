package i18n

import "time"

// Date preserves the local instant and uses a Chinese date layout where the
// English UI contains month/day names. Currency and business values stay intact.
func Date(language string, at time.Time, layout string) string {
	if Normalize(language) == Chinese {
		switch layout {
		case "Mon":
			return [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[at.Weekday()]
		case "Jan 2":
			layout = "1月2日"
		case "Jan 2, 2006 15:04":
			layout = "2006年1月2日 15:04"
		case "Jan 2, 15:04", "Jan 2, 3:04 PM":
			layout = "1月2日 15:04"
		case "Mon 3:04 PM", "3:04 PM":
			layout = "15:04"
		}
	}
	return at.Format(layout)
}

// Duration formats UI elapsed time in whole seconds, retaining every lower
// unit through seconds. Measurement and persistence keep their original precision.
func Duration(language string, milliseconds int64) string {
	seconds := max(milliseconds/1000, 0)
	switch {
	case seconds < 60:
		return In(language, "%ds", seconds)
	case seconds < 3600:
		return In(language, "%dm %02ds", seconds/60, seconds%60)
	case seconds < 86400:
		return In(language, "%dh %02dm %02ds", seconds/3600, seconds/60%60, seconds%60)
	default:
		return In(language, "%dd %02dh %02dm %02ds", seconds/86400, seconds/3600%24, seconds/60%60, seconds%60)
	}
}
