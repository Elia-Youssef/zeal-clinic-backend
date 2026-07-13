package conv

import (
	"strings"
	"unicode"
)

func HasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

type Name struct {
	First, Middle, Last string
}

func ParseFullName(full string) Name {
	fields := strings.Fields(full)
	toks := fields[:0]
	for _, t := range fields {
		if t == "." || t == "-" {
			continue
		}
		toks = append(toks, t)
	}
	switch len(toks) {
	case 0:
		return Name{}
	case 1:
		return Name{First: toks[0]}
	case 2:
		return Name{First: toks[0], Last: toks[1]}
	default:
		return Name{First: toks[0], Middle: strings.Join(toks[1:len(toks)-1], " "), Last: toks[len(toks)-1]}
	}
}

func NormalizeGender(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "m", "male":
		return "Male"
	case "f", "female":
		return "Female"
	default:
		return ""
	}
}

var bloodTypeFix = map[string]string{
	"0+": "O+", "0-": "O-",
	"o+": "O+", "o-": "O-",
	"a+": "A+", "a-": "A-",
	"b+": "B+", "b-": "B-",
	"ab+": "AB+", "ab-": "AB-",
}

var validBloodTypes = map[string]bool{
	"A+": true, "A-": true, "B+": true, "B-": true,
	"O+": true, "O-": true, "AB+": true, "AB-": true,
}

func NormalizeBloodType(v string) (clean string, unrecognized bool) {
	t := strings.TrimSpace(v)
	if t == "" || t == "-" {
		return "", false
	}
	if fixed, ok := bloodTypeFix[strings.ToLower(t)]; ok {
		return fixed, false
	}
	up := strings.ToUpper(t)
	if validBloodTypes[up] {
		return up, false
	}
	return "", true
}
