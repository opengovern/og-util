package source

import (
	"fmt"
	"strings"
)

type Type string

const (
	Nil Type = ""

	CloudAWS   Type = "AWS"
	CloudAzure Type = "Azure"
)

var (
	List = []Type{CloudAWS, CloudAzure}
)

func ParseType(str string) (Type, error) {
	str = strings.ToLower(str)
	switch str {
	case "aws":
		return CloudAWS, nil
	case "azure":
		return CloudAzure, nil
	default:
		return "", fmt.Errorf("invalid provider: %s", str)
	}
}

func ParseTypes(str []string) []Type {
	result := make([]Type, 0, len(str))
	for _, s := range str {
		t, err := ParseType(s)
		if err != nil || t == Nil {
			continue
		}
		result = append(result, t)
	}
	return result
}

func (t Type) AsStringPtr() *string {
	if t == "" {
		return nil
	}
	v := string(t)
	return &v
}

func (t Type) AsPtr() *Type {
	if t == "" {
		return nil
	}
	return &t
}

func (t Type) IsNull() bool {
	return t == ""
}

func (t Type) String() string {
	return string(t)
}
