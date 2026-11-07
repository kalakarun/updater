package release

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const MaxArchiveBytes int64 = 1073741824

type Version int

func ParseVersion(text string) (Version, error) {
	parts := strings.SplitN(text, ".", 4)
	if len(parts) != 3 || len(parts[0]) == 0 || parts[0][0] == '0' {
		return 0, errors.New("version must be major.minor.patch with major 1..99 and minor/patch 0..99")
	}
	var components [3]int
	for index, part := range parts {
		if len(part) < 1 || len(part) > 2 || !decimalDigits(part) {
			return 0, errors.New("version must be major.minor.patch with major 1..99 and minor/patch 0..99")
		}
		for _, digit := range []byte(part) {
			components[index] = components[index]*10 + int(digit-'0')
		}
	}
	return Version(components[0]*10000 + components[1]*100 + components[2]), nil
}

func ParseInstalled(data []byte) (Version, error) {
	text := strings.TrimSpace(string(data))
	if len(text) > 16 || !decimalDigits(text) {
		return 0, errors.New("swversion must contain digits only")
	}
	number, err := strconv.ParseUint(text, 10, 32)
	if err != nil || number < 10000 || number > 999999 {
		return 0, errors.New("swversion outside supported range")
	}
	return Version(number), nil
}

func decimalDigits(text string) bool {
	if len(text) == 0 {
		return false
	}
	for index := range text {
		if text[index] < '0' || text[index] > '9' {
			return false
		}
	}
	return true
}

func ValidSHA256(text string) bool {
	if len(text) != 64 {
		return false
	}
	for index := range text {
		if (text[index] < '0' || text[index] > '9') && (text[index] < 'a' || text[index] > 'f') {
			return false
		}
	}
	return true
}

func (version Version) String() string {
	return fmt.Sprintf("%d.%02d.%02d", version/10000, version/100%100, version%100)
}

func (version Version) Installed(previous []byte) []byte {
	text := strconv.Itoa(int(version))
	if strings.HasSuffix(string(previous), "\r\n") {
		text += "\r\n"
	} else if strings.HasSuffix(string(previous), "\n") {
		text += "\n"
	}
	return []byte(text)
}
