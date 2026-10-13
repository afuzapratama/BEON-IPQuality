package scoring

import (
	"math"
	"sort"
	"time"

	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// PolicyVersion names the conservative heuristic, not a calibrated probability.
const PolicyVersion = "evidence-family-max-v1"

// Evidence retains the strongest decayed signal per correlated evidence family.
// Infrastructure is context, capped at 20, and cannot by itself cross the
// default risky threshold. ASN metadata joins context rather than adding twice.
func (s *Scorer) Evidence(threats []models.Threat, asn *models.ASNInfo, now time.Time) map[string]int {
	evidence := make(map[string]int)
	for _, threat := range threats {
		weight := s.sourceWeight(threat.Source, threat.Weight, threat.ThreatType)
		confidence := threat.Confidence
		if confidence <= 0 {
			confidence = 0.5
		}
		confidence = math.Min(confidence, 1)
		value := int(math.Round(float64(weight) * confidence * s.calculateDecay(threat.LastSeen, now) * 100))
		family := evidenceFamily(threat.ThreatType)
		cap := 10000
		if family == "infrastructure" {
			cap = 2000
		}
		value = max(0, min(value, cap))
		if value > evidence[family] {
			evidence[family] = value
		}
	}
	if len(evidence) > 0 && asn != nil {
		context := max(0, min(s.getASNModifier(asn.ASNType)*100, 2000))
		evidence["infrastructure"] = max(evidence["infrastructure"], context)
	}
	return evidence
}

func evidenceFamily(threatType string) string {
	switch threatType {
	case "datacenter", "hosting":
		return "infrastructure"
	case "tor", "vpn", "proxy", "anonymous":
		return "anonymity"
	default:
		return "abuse"
	}
}

// ScoreEvidence combines distinct families with a bounded complement product.
// Sorting keeps floating-point rounding deterministic across map iteration.
// This is an operational risk score, never a probability of maliciousness.
func ScoreEvidence(evidence map[string]int, minScore, maxScore int) int {
	keys := make([]string, 0, len(evidence))
	for key := range evidence {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	remaining := 1.0
	for _, key := range keys {
		remaining *= 1 - float64(max(0, min(evidence[key], 10000)))/10000
	}
	return max(minScore, min(int(math.Round(100*(1-remaining))), maxScore))
}

func (s *Scorer) MaxScore() int { return s.config.MaxScore }
