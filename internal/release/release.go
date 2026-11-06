package release

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const MaxArchiveBytes int64 = 1073741824

var versionPattern = regexp.MustCompile(`^([1-9][0-9]?)\.([0-9]{1,2})\.([0-9]{1,2})$`)
var digitsPattern = regexp.MustCompile(`^[0-9]+$`)

type Version int

func ParseVersion(text string) (Version, error) {
	parts := versionPattern.FindStringSubmatch(text)
	if parts == nil {
		return 0, errors.New("version must be major.minor.patch with major 1..99 and minor/patch 0..99")
	}
	major, _ := strconv.Atoi(parts[1])
	minor, _ := strconv.Atoi(parts[2])
	patch, _ := strconv.Atoi(parts[3])
	return Version(major*10000 + minor*100 + patch), nil
}

func ParseInstalled(data []byte) (Version, error) {
	text := strings.TrimSpace(string(data))
	if !digitsPattern.MatchString(text) || len(text) > 16 {
		return 0, errors.New("swversion must contain digits only")
	}
	number, err := strconv.ParseUint(text, 10, 32)
	if err != nil || number < 10000 || number > 999999 {
		return 0, errors.New("swversion outside supported range")
	}
	return Version(number), nil
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
