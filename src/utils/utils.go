package utils

import "regexp"

func SplitAddress(address string) (string, string) {
	var re = regexp.MustCompile(`(?m)^([\w\-\_\.]+?)\:(\d+)$`)
	return re.ReplaceAllString(address, "$1"), re.ReplaceAllString(address, "$2")
}
