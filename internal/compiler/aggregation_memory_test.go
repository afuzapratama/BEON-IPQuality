package compiler

import (
	"fmt"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/internal/scoring"
	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestGroupedAggregationMatchesReference(t *testing.T) {
	now := time.Now().UTC()
	c := &Compiler{scorer: scoring.NewDefault()}
	input := []models.IPReputation{}
	ranges := []string{"8.8.8.1/24", "8.8.8.0/24", "8.8.8.8", "8.8.8.8/32", "2001:4860::1/64", "2001:4860::/64", "invalid"}
	threats := []string{"proxy", "malware", "datacenter", "tor"}
	for i := 0; i < 400; i++ {
		input = append(input, models.IPReputation{IPRange: ranges[i%len(ranges)], Source: fmt.Sprintf("source%d", i%5), ThreatType: threats[i%4], Confidence: 0.8, Weight: 40 + i%3, LastSeen: now.Add(-time.Duration(i) * time.Hour)})
	}
	preserved := append([]models.IPReputation(nil), input...)
	want, err := c.aggregateReputationsReference(input, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.aggregateReputations(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("grouped output differs from reference")
	}
	if !reflect.DeepEqual(input, preserved) {
		t.Fatal("input was mutated")
	}
}

func BenchmarkAggregation(b *testing.B) {
	now := time.Now().UTC()
	c := &Compiler{scorer: scoring.NewDefault()}
	rows := make([]models.IPReputation, 10000)
	for i := range rows {
		rows[i] = models.IPReputation{IPRange: fmt.Sprintf("11.%d.%d.%d/32", i>>16, (i>>8)&255, i&255), Source: "proxy/source", ThreatType: "proxy", Confidence: 0.8, Weight: 40, LastSeen: now}
	}
	for _, name := range []string{"reference", "grouped"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				if name == "reference" {
					_, err = c.aggregateReputationsReference(rows, now)
				} else {
					_, err = c.aggregateReputations(rows, now)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func (c *Compiler) aggregateReputationsReference(reputations []models.IPReputation, now time.Time) ([]mmdb.ReputationEntry, error) {
	aggregates := make(map[string]*reputationAggregate)
	for _, reputation := range reputations {
		prefix, err := parseReputationPrefix(reputation.IPRange)
		if err != nil {
			logger.Warn(fmt.Sprintf("Skipping invalid reputation range %q: %v", reputation.IPRange, err))
			continue
		}
		prefix = prefix.Masked()
		key := prefix.String()
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &reputationAggregate{
				prefix:      prefix,
				sources:     make(map[string]struct{}),
				threatTypes: make(map[string]struct{}),
			}
			aggregates[key] = aggregate
		}

		aggregate.threats = append(aggregate.threats, models.Threat{
			ThreatType: reputation.ThreatType,
			Source:     reputation.Source,
			Confidence: reputation.Confidence,
			Weight:     reputation.Weight,
			LastSeen:   reputation.LastSeen,
		})
		aggregate.sources[reputation.Source] = struct{}{}
		aggregate.threatTypes[reputation.ThreatType] = struct{}{}
		aggregate.flags = mergeFlags(aggregate.flags, flagsForThreatType(reputation.ThreatType))
		primaryValue := float64(reputation.Weight) * reputation.Confidence
		if primaryValue > aggregate.primaryValue {
			aggregate.primaryValue = primaryValue
			aggregate.primaryType = reputation.ThreatType
		}
		if reputation.Confidence > aggregate.confidence {
			aggregate.confidence = reputation.Confidence
		}
		if reputation.LastSeen.After(aggregate.lastUpdate) {
			aggregate.lastUpdate = reputation.LastSeen
		}
	}

	keys := make([]string, 0, len(aggregates))
	for key := range aggregates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entries := make([]mmdb.ReputationEntry, 0, len(keys))
	for _, key := range keys {
		aggregate := aggregates[key]
		sources := mapKeys(aggregate.sources)
		threatTypes := mapKeys(aggregate.threatTypes)
		asn, err := c.classifyASN(aggregate.prefix)
		if err != nil {
			return nil, err
		}
		if asn != nil && (asn.ASNType == "datacenter" || asn.ASNType == "hosting") {
			aggregate.flags.IsDatacenter = true
		}
		score := c.scorer.CalculateScore(aggregate.threats, asn, now)
		entries = append(entries, mmdb.ReputationEntry{
			Prefix:        aggregate.prefix,
			RiskScore:     score,
			Evidence:      c.scorer.Evidence(aggregate.threats, asn, now),
			ScoreCap:      c.scorer.MaxScore(),
			RiskThreshold: c.riskThreshold(),
			RiskLevel:     c.scorer.ClassifyRisk(score),
			ThreatType:    aggregate.primaryType,
			ThreatTypes:   threatTypes,
			Confidence:    aggregate.confidence,
			Sources:       sources,
			Flags:         aggregate.flags,
			LastUpdate:    aggregate.lastUpdate,
			ASN:           asn,
		})
	}
	return entries, nil
}
