package dataaccess

import (
	"database/sql/driver"
	"fmt"
)

// FormatValue normalizes a database driver value to a string.
func FormatValue(val any) string {
	if val == nil {
		return ""
	}
	if valuer, ok := val.(driver.Valuer); ok {
		v, err := valuer.Value()
		if err == nil && v != nil {
			switch ts := v.(type) {
			case []byte:
				return string(ts)
			default:
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return fmt.Sprintf("%v", val)
}
