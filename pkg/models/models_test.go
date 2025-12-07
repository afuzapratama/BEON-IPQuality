package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIPReputationJSON(t *testing.T) {
	rep := IPReputation{
		ID:         1,
		IPRange:    "192.168.1.0/24",
		Source:     "test_source",
		ThreatType: "proxy",
		Confidence: 0.85,
		Weight:     50,
		RiskScore:  65,
		FirstSeen:  time.Now().Add(-24 * time.Hour),
		LastSeen:   time.Now(),
	}

	// Test JSON marshaling
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("Failed to marshal IPReputation: %v", err)
	}

	// Test JSON unmarshaling
	var decoded IPReputation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal IPReputation: %v", err)
	}

	if decoded.IPRange != rep.IPRange {
		t.Errorf("IPRange mismatch: got %s, want %s", decoded.IPRange, rep.IPRange)
	}
	if decoded.ThreatType != rep.ThreatType {
		t.Errorf("ThreatType mismatch: got %s, want %s", decoded.ThreatType, rep.ThreatType)
	}
}

func TestIPCheckResultJSON(t *testing.T) {
	result := IPCheckResult{
		IP:           "8.8.8.8",
		Score:        45,
		RiskScore:    45,
		RiskLevel:    "medium",
		IsProxy:      true,
		IsVPN:        false,
		IsTor:        false,
		IsDatacenter: true,
		QueryTime:    0.5,
		Cached:       false,
		Geo: &GeoInfo{
			Country:     "United States",
			CountryCode: "US",
			City:        "Mountain View",
		},
		ASN: &ASNInfo{
			ASN: 15169,
			Org: "Google LLC",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal IPCheckResult: %v", err)
	}

	var decoded IPCheckResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal IPCheckResult: %v", err)
	}

	if decoded.IP != result.IP {
		t.Errorf("IP mismatch: got %s, want %s", decoded.IP, result.IP)
	}
	if decoded.Score != result.Score {
		t.Errorf("Score mismatch: got %d, want %d", decoded.Score, result.Score)
	}
	if decoded.IsProxy != result.IsProxy {
		t.Errorf("IsProxy mismatch: got %v, want %v", decoded.IsProxy, result.IsProxy)
	}
	if decoded.Geo == nil || decoded.Geo.Country != result.Geo.Country {
		t.Error("Geo info not properly decoded")
	}
	if decoded.ASN == nil || decoded.ASN.ASN != result.ASN.ASN {
		t.Error("ASN info not properly decoded")
	}
}

func TestGetRiskLevel(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{0, "safe"},
		{10, "safe"},
		{24, "safe"},
		{25, "low"},
		{49, "low"},
		{50, "medium"},
		{69, "medium"},
		{70, "high"},
		{84, "high"},
		{85, "critical"},
		{100, "critical"},
	}

	for _, tt := range tests {
		got := GetRiskLevel(tt.score)
		if got != tt.want {
			t.Errorf("GetRiskLevel(%d) = %v, want %v", tt.score, got, tt.want)
		}
	}
}

func TestFeedEntryIsPrefix(t *testing.T) {
	tests := []struct {
		name     string
		entry    FeedEntry
		isPrefix bool
	}{
		{
			name: "Single IP",
			entry: FeedEntry{
				IPString: "192.168.1.1",
			},
			isPrefix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.IsPrefix(); got != tt.isPrefix {
				t.Errorf("IsPrefix() = %v, want %v", got, tt.isPrefix)
			}
		})
	}
}

func TestBatchCheckRequest(t *testing.T) {
	req := BatchCheckRequest{
		IPs: []string{"8.8.8.8", "1.1.1.1", "192.168.1.1"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal BatchCheckRequest: %v", err)
	}

	var decoded BatchCheckRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal BatchCheckRequest: %v", err)
	}

	if len(decoded.IPs) != len(req.IPs) {
		t.Errorf("IPs length mismatch: got %d, want %d", len(decoded.IPs), len(req.IPs))
	}
}

func TestThreatJSON(t *testing.T) {
	threat := Threat{
		Type:       "tor",
		ThreatType: "tor",
		Source:     "tor_exit",
		Confidence: 0.95,
		Weight:     70,
		LastSeen:   time.Now(),
	}

	data, err := json.Marshal(threat)
	if err != nil {
		t.Fatalf("Failed to marshal Threat: %v", err)
	}

	var decoded Threat
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Threat: %v", err)
	}

	if decoded.ThreatType != threat.ThreatType {
		t.Errorf("ThreatType mismatch: got %s, want %s", decoded.ThreatType, threat.ThreatType)
	}
	if decoded.Confidence != threat.Confidence {
		t.Errorf("Confidence mismatch: got %f, want %f", decoded.Confidence, threat.Confidence)
	}
}

// Benchmark JSON operations
func BenchmarkIPCheckResultMarshal(b *testing.B) {
	result := IPCheckResult{
		IP:           "8.8.8.8",
		Score:        45,
		RiskLevel:    "medium",
		IsProxy:      true,
		IsDatacenter: true,
		QueryTime:    0.5,
		Geo: &GeoInfo{
			Country:     "United States",
			CountryCode: "US",
		},
		ASN: &ASNInfo{
			ASN: 15169,
			Org: "Google LLC",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(result)
	}
}

func BenchmarkIPCheckResultUnmarshal(b *testing.B) {
	data := []byte(`{"ip":"8.8.8.8","score":45,"risk_level":"medium","proxy":true,"datacenter":true}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result IPCheckResult
		json.Unmarshal(data, &result)
	}
}
