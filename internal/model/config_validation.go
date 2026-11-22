package model

import (
	"net"
	"regexp"
	"strings"
	"unicode"
)

var hostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

func ValidSwitchHost(host string) bool {
	return net.ParseIP(host) != nil || hostName.MatchString(host)
}

func ValidGroupName(name string) bool {
	return name != "" && name != "*" && name != "." && name != ".." && strings.IndexFunc(name, unicode.IsControl) == -1
}
