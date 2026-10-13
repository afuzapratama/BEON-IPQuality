package mmdb

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"

	"github.com/lfrfrfr/beon-ipquality/internal/scoring"
	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// WriterConfig holds configuration for MMDB writing
type WriterConfig struct {
	Feeds               []models.FeedFreshness
	DatabaseType        string
	Description         string
	RecordSize          int // 24, 28, or 32
	IPVersion           int // 4, 6, or 0 for both
	IncludeReservedNets bool
	DisableIPv4Aliasing bool
}

// DefaultWriterConfig returns the default writer configuration
func DefaultWriterConfig() WriterConfig {
	return WriterConfig{
		DatabaseType:        "BEON-IPReputation",
		Description:         "BEON IP Reputation Database",
		RecordSize:          28,
		IPVersion:           0, // Both IPv4 and IPv6
		IncludeReservedNets: false,
		DisableIPv4Aliasing: false,
	}
}

// Writer handles writing MMDB files
type Writer struct {
	config WriterConfig
}

// NewWriter creates a new MMDB writer
func NewWriter(config WriterConfig) *Writer {
	return &Writer{config: config}
}

// NewDefaultWriter creates a writer with default configuration
func NewDefaultWriter() *Writer {
	return NewWriter(DefaultWriterConfig())
}

// ReputationEntry represents a single entry to write
type ReputationEntry struct {
	Evidence      map[string]int
	ScoreCap      int
	RiskThreshold int
	ASN           *models.ASNInfo
	Prefix        netip.Prefix
	RiskScore     int
	RiskLevel     string
	ThreatType    string
	ThreatTypes   []string
	Confidence    float64
	Sources       []string
	Flags         EntryFlags
	LastUpdate    time.Time
	Whitelisted   bool
}

// EntryFlags represents boolean threat flags
type EntryFlags struct {
	IsTor        bool
	IsVPN        bool
	IsProxy      bool
	IsDatacenter bool
	IsBotnet     bool
	IsMalware    bool
	IsSpam       bool
	IsAttacker   bool
}

// CompileToMMDB compiles reputation entries to an MMDB file
func (w *Writer) CompileToMMDB(entries []ReputationEntry, outputPath string) error {
	logger.Info(fmt.Sprintf("Starting MMDB compilation with %d entries", len(entries)))
	startTime := time.Now()

	// Create output directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	description := map[string]string{"en": w.config.Description}
	if w.config.Feeds != nil {
		data, err := json.Marshal(w.config.Feeds)
		if err != nil {
			return err
		}
		description["beon_feed_freshness"] = string(data)
	}
	description["beon_scoring_policy"] = scoring.PolicyVersion
	// Create MMDB writer
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            w.config.DatabaseType,
		Description:             description,
		RecordSize:              w.config.RecordSize,
		IPVersion:               w.config.IPVersion,
		IncludeReservedNetworks: w.config.IncludeReservedNets,
		DisableIPv4Aliasing:     w.config.DisableIPv4Aliasing,
		Inserter:                inserter.ReplaceWith,
	})
	if err != nil {
		return fmt.Errorf("failed to create MMDB writer: %w", err)
	}

	// Insert entries
	var insertedCount int
	var insertErrors []error

	for _, entry := range entries {
		record := w.entryToMMDBRecord(entry)

		// Convert netip.Prefix to net.IPNet
		if !entry.Prefix.IsValid() {
			insertErrors = append(insertErrors, fmt.Errorf("invalid prefix"))
			continue
		}
		ipNet := prefixToIPNet(entry.Prefix.Masked())

		err := tree.InsertFunc(ipNet, mergeReputationRecord(record))
		if err != nil {
			logger.Debug(fmt.Sprintf("Failed to insert %s: %v", entry.Prefix, err))
			insertErrors = append(insertErrors, fmt.Errorf("%s: %w", entry.Prefix, err))
			continue
		}
		insertedCount++
	}
	if len(insertErrors) > 0 {
		return fmt.Errorf("failed to insert %d of %d MMDB entries: %v", len(insertErrors), len(entries), insertErrors[0])
	}

	// Write to file
	tempPath := outputPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	_, err = tree.WriteTo(file)
	closeErr := file.Close()
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to write MMDB: %w", err)
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close MMDB: %w", closeErr)
	}

	// Atomic rename
	if err := os.Rename(tempPath, outputPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	logger.Info(fmt.Sprintf("MMDB compilation complete: %d entries inserted, took %v",
		insertedCount, time.Since(startTime)))

	return nil
}

// entryToMMDBRecord converts a ReputationEntry to MMDB record format
func (w *Writer) entryToMMDBRecord(entry ReputationEntry) mmdbtype.DataType {
	// Build sources array
	sources := mmdbtype.Slice{}
	for _, s := range entry.Sources {
		sources = append(sources, mmdbtype.String(s))
	}

	threshold := entry.RiskThreshold
	if threshold <= 0 {
		threshold = 50
	}
	record := mmdbtype.Map{
		"risk_threshold": mmdbtype.Uint16(threshold),
		"risk_score":     mmdbtype.Uint16(entry.RiskScore),
		"risk_level":     mmdbtype.String(entry.RiskLevel),
		"threat_type":    mmdbtype.String(entry.ThreatType),
		"confidence":     mmdbtype.Uint16(int(entry.Confidence * 100)),
		"sources":        sources,
		"threat_types":   stringSliceToMMDB(entry.ThreatTypes),
		"last_update":    mmdbtype.Uint64(entry.LastUpdate.Unix()),
		"whitelisted":    mmdbtype.Bool(entry.Whitelisted),
		"is_tor":         mmdbtype.Bool(entry.Flags.IsTor),
		"is_vpn":         mmdbtype.Bool(entry.Flags.IsVPN),
		"is_proxy":       mmdbtype.Bool(entry.Flags.IsProxy),
		"is_datacenter":  mmdbtype.Bool(entry.Flags.IsDatacenter),
		"is_botnet":      mmdbtype.Bool(entry.Flags.IsBotnet),
		"is_malware":     mmdbtype.Bool(entry.Flags.IsMalware),
		"is_spam":        mmdbtype.Bool(entry.Flags.IsSpam),
		"is_attacker":    mmdbtype.Bool(entry.Flags.IsAttacker),
	}

	if entry.Evidence != nil {
		evidence := mmdbtype.Map{}
		for family, value := range entry.Evidence {
			evidence[mmdbtype.String(family)] = mmdbtype.Uint16(value)
		}
		record["risk_evidence"] = evidence
		cap := entry.ScoreCap
		if cap <= 0 {
			cap = 100
		}
		record["risk_max_score"] = mmdbtype.Uint16(cap)
	}
	if entry.ASN != nil {
		record["asn"] = mmdbtype.Uint32(entry.ASN.ASN)
		record["asn_org"] = mmdbtype.String(entry.ASN.Org)
		record["asn_type"] = mmdbtype.String(entry.ASN.ASNType)
	}
	return record
}

func stringSliceToMMDB(values []string) mmdbtype.Slice {
	result := make(mmdbtype.Slice, 0, len(values))
	for _, value := range values {
		result = append(result, mmdbtype.String(value))
	}
	return result
}

// mergeReputationRecord combines records when feeds contain the same or
// overlapping prefixes. A whitelist record always wins over threat data.
func mergeReputationRecord(newValue mmdbtype.DataType) inserter.Func {
	return func(existingValue mmdbtype.DataType) (mmdbtype.DataType, error) {
		newMap, ok := newValue.(mmdbtype.Map)
		if !ok {
			return nil, fmt.Errorf("new reputation record is %T, expected mmdbtype.Map", newValue)
		}
		if existingValue == nil {
			return newMap.Copy(), nil
		}
		existingMap, ok := existingValue.(mmdbtype.Map)
		if !ok {
			return nil, fmt.Errorf("existing reputation record is %T, expected mmdbtype.Map", existingValue)
		}

		if boolValue(newMap, "whitelisted") {
			return newMap.Copy(), nil
		}
		if boolValue(existingMap, "whitelisted") {
			return existingMap.Copy(), nil
		}

		merged := existingMap.Copy().(mmdbtype.Map)
		for _, key := range []mmdbtype.String{"asn", "asn_org", "asn_type"} {
			if value, ok := newMap[key]; ok {
				merged[key] = value.Copy()
			}
		}
		for _, key := range []string{"is_tor", "is_vpn", "is_proxy", "is_datacenter", "is_botnet", "is_malware", "is_spam", "is_attacker"} {
			merged[mmdbtype.String(key)] = mmdbtype.Bool(boolValue(existingMap, key) || boolValue(newMap, key))
		}
		for _, key := range []string{"sources", "threat_types"} {
			merged[mmdbtype.String(key)] = mergeStringSlices(existingMap[mmdbtype.String(key)], newMap[mmdbtype.String(key)])
		}

		oldScore := uint16Value(existingMap, "risk_score")
		newScore := uint16Value(newMap, "risk_score")
		combinedScore := max(oldScore, newScore)
		oldEvidence, oldOK := existingMap["risk_evidence"].(mmdbtype.Map)
		newEvidence, newOK := newMap["risk_evidence"].(mmdbtype.Map)
		if oldOK && newOK {
			evidence := oldEvidence.Copy().(mmdbtype.Map)
			for family, value := range newEvidence {
				previous, _ := evidence[family].(mmdbtype.Uint16)
				incoming, _ := value.(mmdbtype.Uint16)
				if incoming > previous {
					evidence[family] = incoming
				}
			}
			values := make(map[string]int, len(evidence))
			for family, value := range evidence {
				v, _ := value.(mmdbtype.Uint16)
				values[string(family)] = int(v)
			}
			cap := min(uint16Value(existingMap, "risk_max_score"), uint16Value(newMap, "risk_max_score"))
			if cap == 0 {
				cap = 100
			}
			combinedScore = uint16(scoring.ScoreEvidence(values, 0, int(cap)))
			merged["risk_evidence"] = evidence
			merged["risk_max_score"] = mmdbtype.Uint16(cap)
		} else {
			// Legacy records lack provenance. Retain the stronger score instead
			// of adding scores that may already include the same source/ASN.
			delete(merged, "risk_evidence")
			delete(merged, "risk_max_score")
		}

		merged["risk_score"] = mmdbtype.Uint16(combinedScore)
		merged["risk_level"] = mmdbtype.String(classifyRisk(int(combinedScore)))
		if newScore >= oldScore {
			merged["threat_type"] = newMap["threat_type"].Copy()
		}
		if uint16Value(newMap, "confidence") > uint16Value(existingMap, "confidence") {
			merged["confidence"] = newMap["confidence"].Copy()
		}
		if uint64Value(newMap, "last_update") > uint64Value(existingMap, "last_update") {
			merged["last_update"] = newMap["last_update"].Copy()
		}
		return merged, nil
	}
}

func boolValue(record mmdbtype.Map, key string) bool {
	value, _ := record[mmdbtype.String(key)].(mmdbtype.Bool)
	return bool(value)
}

func uint16Value(record mmdbtype.Map, key string) uint16 {
	value, _ := record[mmdbtype.String(key)].(mmdbtype.Uint16)
	return uint16(value)
}

func uint64Value(record mmdbtype.Map, key string) uint64 {
	value, _ := record[mmdbtype.String(key)].(mmdbtype.Uint64)
	return uint64(value)
}

func mergeStringSlices(left, right mmdbtype.DataType) mmdbtype.Slice {
	seen := make(map[mmdbtype.String]struct{})
	merged := mmdbtype.Slice{}
	for _, value := range []mmdbtype.DataType{left, right} {
		slice, _ := value.(mmdbtype.Slice)
		for _, item := range slice {
			str, ok := item.(mmdbtype.String)
			if !ok {
				continue
			}
			if _, exists := seen[str]; exists {
				continue
			}
			seen[str] = struct{}{}
			merged = append(merged, str)
		}
	}
	return merged
}

// prefixToIPNet converts netip.Prefix to *net.IPNet
func prefixToIPNet(prefix netip.Prefix) *net.IPNet {
	addr := prefix.Addr()
	bits := prefix.Bits()

	ip := net.IP(addr.AsSlice())

	var mask net.IPMask
	if addr.Is4() {
		mask = net.CIDRMask(bits, 32)
	} else {
		mask = net.CIDRMask(bits, 128)
	}

	return &net.IPNet{
		IP:   ip,
		Mask: mask,
	}
}

// CompileFromIPReputations compiles from models.IPReputation slice
func (w *Writer) CompileFromIPReputations(reputations []models.IPReputation, outputPath string) error {
	entries := make([]ReputationEntry, 0, len(reputations))

	for _, rep := range reputations {
		prefix, err := netip.ParsePrefix(rep.IPRange)
		if err != nil {
			// Try as single IP
			addr, err := netip.ParseAddr(rep.IPRange)
			if err != nil {
				logger.Debug(fmt.Sprintf("Invalid IP range: %s", rep.IPRange))
				continue
			}
			// Convert single IP to /32 or /128
			if addr.Is4() {
				prefix = netip.PrefixFrom(addr, 32)
			} else {
				prefix = netip.PrefixFrom(addr, 128)
			}
		}

		entry := ReputationEntry{
			Prefix:      prefix,
			RiskScore:   rep.RiskScore,
			RiskLevel:   classifyRisk(rep.RiskScore),
			ThreatType:  rep.ThreatType,
			ThreatTypes: []string{rep.ThreatType},
			Confidence:  rep.Confidence,
			Sources:     []string{rep.Source},
			Flags:       threatTypeToFlags(rep.ThreatType),
			LastUpdate:  rep.LastSeen,
		}

		entries = append(entries, entry)
	}

	return w.CompileToMMDB(entries, outputPath)
}

// classifyRisk returns risk level based on score
func classifyRisk(score int) string {
	switch {
	case score >= 85:
		return "critical"
	case score >= 70:
		return "high"
	case score >= 50:
		return "medium"
	case score >= 25:
		return "low"
	default:
		return "clean"
	}
}

// threatTypeToFlags converts threat type to boolean flags.
// NOTE: A single IP can have multiple threat types; callers should
// OR-merge these flags when aggregating across multiple sources.
func threatTypeToFlags(threatType string) EntryFlags {
	flags := EntryFlags{}

	switch threatType {
	case "tor":
		flags.IsTor = true
	case "vpn", "anonymous":
		flags.IsVPN = true
	case "proxy":
		flags.IsProxy = true
	case "datacenter", "hosting":
		flags.IsDatacenter = true
	case "botnet_c2", "botnet":
		flags.IsBotnet = true
	case "malware", "malicious":
		flags.IsMalware = true
	case "spam":
		flags.IsSpam = true
	case "attack", "hijacked", "suspicious":
		flags.IsAttacker = true
	}

	return flags
}

// MergeAndCompile merges multiple reputation sources and compiles to MMDB
func (w *Writer) MergeAndCompile(sources map[string][]models.IPReputation, outputPath string) error {
	// Merge entries by IP, keeping highest risk scores
	merged := make(map[string]ReputationEntry)

	for sourceName, reputations := range sources {
		for _, rep := range reputations {
			key := rep.IPRange

			existing, exists := merged[key]
			if !exists {
				prefix, err := parseToPrefix(rep.IPRange)
				if err != nil {
					continue
				}

				merged[key] = ReputationEntry{
					Prefix:      prefix,
					RiskScore:   rep.RiskScore,
					RiskLevel:   classifyRisk(rep.RiskScore),
					ThreatType:  rep.ThreatType,
					ThreatTypes: []string{rep.ThreatType},
					Confidence:  rep.Confidence,
					Sources:     []string{sourceName},
					Flags:       threatTypeToFlags(rep.ThreatType),
					LastUpdate:  rep.LastSeen,
				}
			} else {
				// Merge: keep higher score, combine sources
				if rep.RiskScore > existing.RiskScore {
					existing.RiskScore = rep.RiskScore
					existing.RiskLevel = classifyRisk(rep.RiskScore)
				}
				if rep.Confidence > existing.Confidence {
					existing.Confidence = rep.Confidence
				}
				// Add source if not already present
				sourceExists := false
				for _, s := range existing.Sources {
					if s == sourceName {
						sourceExists = true
						break
					}
				}
				if !sourceExists {
					existing.Sources = append(existing.Sources, sourceName)
				}
				// Merge flags
				newFlags := threatTypeToFlags(rep.ThreatType)
				existing.Flags.IsTor = existing.Flags.IsTor || newFlags.IsTor
				existing.Flags.IsVPN = existing.Flags.IsVPN || newFlags.IsVPN
				existing.Flags.IsProxy = existing.Flags.IsProxy || newFlags.IsProxy
				existing.Flags.IsDatacenter = existing.Flags.IsDatacenter || newFlags.IsDatacenter
				existing.Flags.IsBotnet = existing.Flags.IsBotnet || newFlags.IsBotnet
				existing.Flags.IsMalware = existing.Flags.IsMalware || newFlags.IsMalware
				existing.Flags.IsSpam = existing.Flags.IsSpam || newFlags.IsSpam
				existing.Flags.IsAttacker = existing.Flags.IsAttacker || newFlags.IsAttacker
				if !containsString(existing.ThreatTypes, rep.ThreatType) {
					existing.ThreatTypes = append(existing.ThreatTypes, rep.ThreatType)
				}

				if rep.LastSeen.After(existing.LastUpdate) {
					existing.LastUpdate = rep.LastSeen
				}

				merged[key] = existing
			}
		}
	}

	// Convert map to slice
	entries := make([]ReputationEntry, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}

	logger.Info(fmt.Sprintf("Merged %d unique IP ranges from %d sources", len(entries), len(sources)))

	return w.CompileToMMDB(entries, outputPath)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// parseToPrefix parses a string to netip.Prefix
func parseToPrefix(s string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		if addr.Is4() {
			return netip.PrefixFrom(addr, 32), nil
		}
		return netip.PrefixFrom(addr, 128), nil
	}
	return prefix, nil
}

func (w *Writer) SetFeedFreshness(feeds []models.FeedFreshness) { w.config.Feeds = feeds }
