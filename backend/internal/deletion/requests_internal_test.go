package deletion

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

func TestValidSubject(t *testing.T) {
	const install = "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f"
	cases := []struct {
		name      string
		typ       string
		value     string
		wantType  sqlc.DeletionSubject
		wantValue string
		field     string // "" — значение принято
	}{
		{"install", "install", install, sqlc.DeletionSubjectInstall, install, ""},
		{"install_upper_is_lowered", "install", strings.ToUpper(install), sqlc.DeletionSubjectInstall, install, ""},
		{"install_not_uuid", "install", "abc", "", "", "subject_value"},
		{"install_nil", "install", "00000000-0000-0000-0000-000000000000", "", "", "subject_value"},
		{"install_braced", "install", "{" + install + "}", "", "", "subject_value"},
		{"install_urn", "install", "urn:uuid:" + install, "", "", "subject_value"},
		{"install_no_hyphens", "install", strings.ReplaceAll(install, "-", ""), "", "", "subject_value"},
		{"user", "user", "42", sqlc.DeletionSubjectUser, "42", ""},
		{"user_trimmed", "user", "  a/b c\n", sqlc.DeletionSubjectUser, "a/b c", ""},
		{"user_128_runes", "user", strings.Repeat("я", 128), sqlc.DeletionSubjectUser, strings.Repeat("я", 128), ""},
		{"user_129_runes", "user", strings.Repeat("я", 129), "", "", "subject_value"},
		{"user_empty", "user", "", "", "", "subject_value"},
		{"user_blank", "user", " \t ", "", "", "subject_value"},
		{"app", "app", install, "", "", "subject_type"},
		{"empty_type", "", "u1", "", "", "subject_type"},
		{"type_case_sensitive", "User", "u1", "", "", "subject_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ, value, err := validSubject(tc.typ, tc.value)
			if tc.field == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantType, typ)
				require.Equal(t, tc.wantValue, value)
				return
			}
			var verr *ValidationError
			require.ErrorAs(t, err, &verr)
			require.Equal(t, tc.field, verr.Field)
			require.NotEmpty(t, verr.Message)
		})
	}
}
