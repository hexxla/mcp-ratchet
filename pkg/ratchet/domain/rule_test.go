package domain

import (
	"testing"
	"time"
)

func TestRuleValidate(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{"valid rule", Rule{Tool: "Create Cell", Prerequisite: "List Tags", Expiry: 5 * time.Minute}, false},
		{"empty tool", Rule{Tool: "", Prerequisite: "List Tags"}, true},
		{"empty prerequisite", Rule{Tool: "Create Cell", Prerequisite: "", Expiry: 5 * time.Minute}, false},
		{"self dependency", Rule{Tool: "Create Cell", Prerequisite: "Create Cell"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rule.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Rule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
