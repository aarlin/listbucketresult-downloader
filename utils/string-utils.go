package utils

import (
	"regexp"
)

func IsRegex(s string) bool {
	_, err := regexp.Compile(s)
	return err == nil
}
