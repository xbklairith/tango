package domain

import (
	"strings"
	"testing"
)

func TestValidateSecretName_Valid(t *testing.T) {
	valid := []string{"GITHUB_TOKEN", "A", "OPENAI_API_KEY", "DB_PASSWORD_2", "X123"}
	for _, name := range valid {
		if err := ValidateSecretName(name); err != nil {
			t.Errorf("ValidateSecretName(%q) = %v, want nil", name, err)
		}
	}
}

func TestValidateSecretName_Invalid(t *testing.T) {
	cases := []struct {
		name string
		desc string
	}{
		{"", "empty string"},
		{"github_token", "lowercase"},
		{"1TOKEN", "starts with digit"},
		{"_TOKEN", "starts with underscore"},
		{"MY TOKEN", "contains spaces"},
		{"MY-TOKEN", "contains hyphen"},
		{strings.Repeat("A", 129), "too long (129 chars)"},
	}
	for _, tc := range cases {
		if err := ValidateSecretName(tc.name); err == nil {
			t.Errorf("ValidateSecretName(%q) [%s] = nil, want error", tc.name, tc.desc)
		}
	}
}

func TestValidateSecretValue_Valid(t *testing.T) {
	cases := []string{
		"12345678",               // exactly 8 chars
		"a-longer-secret-value!", // typical secret
		strings.Repeat("x", 65536), // exactly 64KB
	}
	for _, v := range cases {
		if err := ValidateSecretValue(v); err != nil {
			t.Errorf("ValidateSecretValue(len=%d) = %v, want nil", len(v), err)
		}
	}
}

func TestValidateSecretValue_Invalid(t *testing.T) {
	cases := []struct {
		value string
		desc  string
	}{
		{"", "empty"},
		{"1234567", "too short (7 chars)"},
		{strings.Repeat("x", 65537), "too long (64KB+1)"},
	}
	for _, tc := range cases {
		if err := ValidateSecretValue(tc.value); err == nil {
			t.Errorf("ValidateSecretValue [%s] = nil, want error", tc.desc)
		}
	}
}

func TestValidateCreateSecretInput(t *testing.T) {
	// Valid
	if err := ValidateCreateSecretInput("GITHUB_TOKEN", "ghp_12345678"); err != nil {
		t.Errorf("ValidateCreateSecretInput valid = %v, want nil", err)
	}

	// Invalid name
	if err := ValidateCreateSecretInput("bad", "ghp_12345678"); err == nil {
		t.Error("ValidateCreateSecretInput with bad name = nil, want error")
	}

	// Invalid value
	if err := ValidateCreateSecretInput("GOOD_NAME", "short"); err == nil {
		t.Error("ValidateCreateSecretInput with short value = nil, want error")
	}
}

func TestMaskValue(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"ghp_abcdefghij1234", "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u20221234"},
		{"12345678", "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u20225678"},
		{"abcdefghijklmnop", "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022mnop"},
	}
	for _, tc := range cases {
		got := MaskValue(tc.input)
		if got != tc.want {
			t.Errorf("MaskValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
