package repository

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	adminAlertMaxLimit       = 100
	adminAlertMaxCondition   = 32 << 10
	adminIncidentMaxLimit    = 100
	adminIncidentMaxServices = 32
	adminDashboardMaxLimit   = 100
	adminDashboardMaxDef     = 256 << 10
)

var (
	ErrInvalidAdminAlert     = errors.New("invalid admin alert")
	ErrInvalidAdminIncident  = errors.New("invalid admin incident")
	ErrInvalidAdminDashboard = errors.New("invalid admin dashboard")
	ErrInvalidAdminStatus    = errors.New("invalid admin status page")
)

func normalizeAdminControlText(value string, minimum, maximum int) (string, error) {
	value = strings.TrimSpace(value)
	if !validAdminControlText(value, minimum, maximum) {
		return "", errors.New("invalid text")
	}
	return value, nil
}

func validAdminControlText(value string, minimum, maximum int) bool {
	length := utf8.RuneCountInString(value)
	return length >= minimum && length <= maximum && !strings.ContainsAny(value, "\x00\r\n")
}
