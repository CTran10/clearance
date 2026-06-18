package domain

import "testing"

func TestEvaluateRiskUsesSimpleAmountRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		amountCents  int64
		wantLevel    RiskLevel
		wantApproved bool
	}{
		{
			name:         "amount at threshold is low risk",
			amountCents:  50_000,
			wantLevel:    RiskLow,
			wantApproved: true,
		},
		{
			name:         "amount above threshold is high risk",
			amountCents:  50_001,
			wantLevel:    RiskHigh,
			wantApproved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := EvaluateRisk(tt.amountCents)

			if got.Level != tt.wantLevel {
				t.Fatalf("level = %q, want %q", got.Level, tt.wantLevel)
			}
			if got.Approved != tt.wantApproved {
				t.Fatalf("approved = %v, want %v", got.Approved, tt.wantApproved)
			}
			if got.Reason == "" {
				t.Fatal("reason should be populated")
			}
		})
	}
}
