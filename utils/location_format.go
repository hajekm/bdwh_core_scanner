package utils

import "regexp"

func IsValidLocationFormat(s string) bool {
	re := regexp.MustCompile(`^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)?-(?:[0-9]|[1-9][0-9])-[A-Za-z0-9]+$`)
	return re.MatchString(s)
}
