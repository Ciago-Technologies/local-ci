package fingerprint

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SARIF output for `localci diff --sarif`.
//
// One SARIF result per difference: rule id from the diff field
// (e.g. env:STRIPE_KEY -> env-STRIPE_KEY), level error for
// critical, warning for warning, note for info, message = the diff
// detail + fix. Human and --json outputs are untouched.

type sarifReport struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name,omitempty"`
	ShortDescription     sarifMessage       `json:"shortDescription"`
	FullDescription      sarifMessage       `json:"fullDescription,omitempty"`
	DefaultConfiguration sarifConfiguration `json:"defaultConfiguration,omitempty"`
}

type sarifConfiguration struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// SarifLevel maps a LocalCI severity to a SARIF level.
func SarifLevel(severity string) string {
	switch severity {
	case SeverityCritical:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "note"
	}
}

// SarifRuleID sanitises a diff field into a SARIF rule id.
// "env:STRIPE_KEY" -> "env-STRIPE_KEY", "runtime:node" -> "runtime-node".
func SarifRuleID(field string) string {
	var b strings.Builder
	for _, r := range field {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// ToSARIF converts diffs to a SARIF 2.1.0 report (JSON bytes).
func ToSARIF(diffs []Difference) ([]byte, error) {
	rulesByID := map[string]sarifRule{}
	var rules []sarifRule
	results := []sarifResult{}

	for _, d := range diffs {
		ruleID := SarifRuleID(d.Field)
		level := SarifLevel(d.Severity)
		if _, ok := rulesByID[ruleID]; !ok {
			rule := sarifRule{
				ID:                   ruleID,
				Name:                 ruleID,
				ShortDescription:     sarifMessage{Text: fmt.Sprintf("LocalCI difference: %s", d.Field)},
				FullDescription:      sarifMessage{Text: fmt.Sprintf("Local (%s) vs CI (%s)", d.A, d.B)},
				DefaultConfiguration: sarifConfiguration{Level: level},
			}
			rulesByID[ruleID] = rule
			rules = append(rules, rule)
		}
		msg := d.Field + ": local = " + d.A + ", CI = " + d.B
		if d.Detail != "" {
			msg += " (" + d.Detail + ")"
		}
		if d.Fix != "" {
			msg += ". Fix: " + d.Fix
		}
		results = append(results, sarifResult{
			RuleID:  ruleID,
			Level:   level,
			Message: sarifMessage{Text: msg},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: "localci-diff"},
					Region:           sarifRegion{StartLine: 1},
				},
			}},
		})
	}
	if results == nil {
		results = []sarifResult{}
	}
	report := sarifReport{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "LocalCI",
				InformationURI: "https://github.com/Ciago-Technologies/local-ci",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	return json.MarshalIndent(report, "", "  ")
}
