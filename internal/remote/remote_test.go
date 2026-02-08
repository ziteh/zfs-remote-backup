package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateStorageClass(t *testing.T) {
	tests := []struct {
		name         string
		storageClass string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "STANDARD is accessible",
			storageClass: "STANDARD",
			wantErr:      false,
		},
		{
			name:         "STANDARD_IA is accessible",
			storageClass: "STANDARD_IA",
			wantErr:      false,
		},
		{
			name:         "INTELLIGENT_TIERING is accessible",
			storageClass: "INTELLIGENT_TIERING",
			wantErr:      false,
		},
		{
			name:         "GLACIER is not accessible",
			storageClass: "GLACIER",
			wantErr:      true,
			errContains:  "not immediately accessible",
		},
		{
			name:         "DEEP_ARCHIVE is not accessible",
			storageClass: "DEEP_ARCHIVE",
			wantErr:      true,
			errContains:  "not immediately accessible",
		},
		{
			name:         "empty string is accessible",
			storageClass: "",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStorageClass(tt.storageClass)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
