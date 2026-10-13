package scoring

import (
	"strings"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
)

// FromConfig applies explicit source overrides before persisted feed weights.
// Both canonical feed IDs and legacy human-readable IDs remain supported.
func FromConfig(cfg config.ScoringConfig, feeds *config.FeedsConfig) *Scorer {
	result := DefaultConfig()
	result.DecayLambda = cfg.DecayLambda
	result.MaxScore = cfg.MaxScore
	result.SourceWeights = make(map[string]int)
	for source, weight := range cfg.Weights {
		result.SourceWeights[source] = weight
	}
	if feeds != nil {
		for id, feed := range feeds.Feeds {
			if weight, ok := cfg.Weights[id]; ok {
				result.SourceWeights[feed.Name] = weight
			}
			for _, source := range feed.Sources {
				if weight, ok := cfg.Weights[id+"/"+source.Name]; ok {
					result.SourceWeights[feed.Name+"/"+source.Name] = weight
				}
			}
		}
	}
	for asnType, bonus := range cfg.ASNBonuses {
		result.ASNTypeModifiers[asnType] = bonus
	}
	return New(result)
}

func (s *Scorer) sourceWeight(source string, persisted int, threatType string) int {
	if weight, ok := s.config.SourceWeights[source]; ok {
		return weight
	}
	if weight, ok := s.config.SourceWeights[strings.SplitN(source, "/", 2)[0]]; ok {
		return weight
	}
	if persisted > 0 {
		return persisted
	}
	return s.getThreatWeight(threatType)
}
