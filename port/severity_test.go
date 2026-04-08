package port

import "testing"

func TestSeverityConstants(t *testing.T) {
	tests := []struct {
		sev  Severity
		want string
	}{
		{SeverityInfo, "info"},
		{SeverityWarning, "warning"},
		{SeverityError, "error"},
		{SeverityCritical, "critical"},
	}
	for _, tt := range tests {
		if string(tt.sev) != tt.want {
			t.Errorf("Severity = %q, want %q", tt.sev, tt.want)
		}
	}
}

func TestRiskLevelConstants(t *testing.T) {
	tests := []struct {
		risk RiskLevel
		want string
	}{
		{RiskLow, "low"},
		{RiskMedium, "medium"},
		{RiskHigh, "high"},
		{RiskCritical, "critical"},
	}
	for _, tt := range tests {
		if string(tt.risk) != tt.want {
			t.Errorf("RiskLevel = %q, want %q", tt.risk, tt.want)
		}
	}
}

func TestConfidenceType(t *testing.T) {
	var c Confidence = 0.85
	if c < 0 || c > 1 {
		t.Errorf("Confidence out of range: %f", c)
	}
}

func TestScoreType(t *testing.T) {
	var s Score = 95.0
	if s < 0 || s > 100 {
		t.Errorf("Score out of range: %f", s)
	}
}
