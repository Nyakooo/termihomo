package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"go.yaml.in/yaml/v3"
)

// CustomRule is stored with its source profile, never in global tool settings.
type CustomRule struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Target    string `json:"target"`
	Disabled  bool   `json:"disabled,omitempty"`
	NoResolve bool   `json:"no_resolve,omitempty"`
}

func (r CustomRule) String() string {
	text := r.Type + "," + r.Value + "," + r.Target
	if r.NoResolve {
		text += ",no-resolve"
	}
	return text
}

// ParseCustomRule preserves the supported IP-CIDR option when copying source rules.
func ParseCustomRule(text string) (CustomRule, error) {
	parts := strings.Split(text, ",")
	invalid := i18n.Message("This rule type or its options cannot be copied in this version")
	if len(parts) < 3 || len(parts) > 4 {
		return CustomRule{}, invalid
	}
	r := CustomRule{Type: parts[0], Value: parts[1], Target: parts[2]}
	switch r.Type {
	case "DOMAIN", "DOMAIN-SUFFIX", "IP-CIDR", "DST-PORT":
	default:
		return CustomRule{}, invalid
	}
	if len(parts) == 4 {
		if r.Type != "IP-CIDR" || parts[3] != "no-resolve" {
			return CustomRule{}, invalid
		}
		r.NoResolve = true
	}
	return r, nil
}

type RoutingRules struct {
	ProfileID string
	Revision  string
	Custom    []CustomRule
	Original  []string
	Targets   []string
}

func profileRules(p profile) (RoutingRules, error) {
	record, err := json.Marshal(p)
	if err != nil {
		return RoutingRules{}, err
	}
	digest := sha256.Sum256(record)
	result := RoutingRules{ProfileID: p.ID, Revision: hex.EncodeToString(digest[:]), Custom: slices.Clone(p.CustomRules), Targets: []string{"DIRECT", "REJECT"}}
	var config struct {
		Groups []struct {
			Name string `yaml:"name"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal(p.YAML, &config); err != nil {
		return result, err
	}
	for _, group := range config.Groups {
		if !slices.Contains(result.Targets, group.Name) {
			result.Targets = append(result.Targets, group.Name)
		}
	}
	result.Original = config.Rules
	if len(result.Original) == 0 {
		result.Original = []string{"MATCH,DIRECT"}
	}
	return result, nil
}

func (m Manager) RoutingRules() (RoutingRules, error) {
	p, err := m.readProfile()
	if err != nil {
		return RoutingRules{}, err
	}
	return profileRules(p)
}

// ValidateCustomRules rejects ambiguous duplicate matchers, even when disabled.
func ValidateCustomRules(rules []CustomRule, targets []string) error {
	seen := map[string]bool{}
	for _, r := range rules {
		if !slices.Contains(targets, r.Target) || strings.ContainsAny(r.Target, ",\r\n") {
			return i18n.Message("Unknown rule target")
		}
		valid := false
		key := r.Type + "," + strings.ToLower(r.Value)
		switch r.Type {
		case "DOMAIN", "DOMAIN-SUFFIX":
			valid = len(r.Value) > 0 && len(r.Value) <= 253
			for _, label := range strings.Split(r.Value, ".") {
				if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
					valid = false
				}
				for _, c := range label {
					if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
						valid = false
					}
				}
			}
		case "IP-CIDR":
			prefix, err := netip.ParsePrefix(r.Value)
			valid = err == nil && prefix.Addr().Is4()
			if valid {
				key = r.Type + "," + prefix.Masked().String()
			}
		case "DST-PORT":
			n, err := strconv.Atoi(r.Value)
			valid = err == nil && n > 0 && n <= 65535 && strconv.Itoa(n) == r.Value
		}
		if !valid || r.NoResolve && r.Type != "IP-CIDR" {
			return i18n.Message("Invalid rule value or type")
		}
		if seen[key] {
			return i18n.Message("Duplicate custom rule")
		}
		seen[key] = true
	}
	return nil
}

func renderProfileWithSettings(p profile, state instance, settings map[string]any) ([]byte, error) {
	// Keep the existing renderer's strict single-document validation.
	body, err := renderConfigWithSettings(p.YAML, state, settings)
	if err != nil || len(p.CustomRules) == 0 {
		return body, err
	}
	rules, err := profileRules(p)
	if err != nil {
		return nil, err
	}
	if err := ValidateCustomRules(p.CustomRules, rules.Targets); err != nil {
		return nil, err
	}
	var config map[string]any
	if err := yaml.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	original := config["rules"].([]any)
	combined := make([]any, 0, len(p.CustomRules)+len(original))
	for _, r := range p.CustomRules {
		if !r.Disabled {
			combined = append(combined, r.String())
		}
	}
	config["rules"] = append(combined, original...)
	return yaml.Marshal(config)
}

func (m Manager) renderProfile(p profile, state instance) ([]byte, error) {
	settings, err := m.readToolSettings()
	if err != nil {
		return nil, err
	}
	return renderProfileWithSettings(p, state, settings)
}

// SaveRoutingRules validates the complete candidate before using the profile
// transaction, including its rollback journal and runtime selector restoration.
func (m Manager) SaveRoutingRules(ctx context.Context, draft RoutingRules) error {
	if err := m.Paths.Ensure(); err != nil {
		return err
	}
	p, err := m.readProfile()
	if err != nil {
		return err
	}
	if p.Source == "" {
		return i18n.Message("No active profile")
	}
	current, err := profileRules(p)
	if err != nil {
		return err
	}
	if current.ProfileID != draft.ProfileID || current.Revision != draft.Revision {
		return i18n.Message("Profile changed during the update. Run the command again.")
	}
	if err := ValidateCustomRules(draft.Custom, current.Targets); err != nil {
		return err
	}
	p.CustomRules = slices.Clone(draft.Custom)
	settings, err := m.validateProfileCandidate(ctx, p)
	if err != nil {
		return err
	}
	return m.withLock(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		latest, err := m.RoutingRules()
		if err != nil {
			return err
		}
		if latest.ProfileID != draft.ProfileID || latest.Revision != draft.Revision {
			return i18n.Message("Profile changed during the update. Run the command again.")
		}
		currentSettings, err := m.readToolSettings()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(settings, currentSettings) {
			return i18n.Message("Settings changed during the update. Run the command again.")
		}
		return m.commitProfileCandidate(ctx, p, true, m.failRoutingChange, true)
	})
}

// Rule edits use hot rollback, like Reload, without restarting the kernel or
// deleting connections. Keep the recovery journal when restoration is incomplete.
func (m Manager) failRoutingChange(state instance, pending profileChange, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	diskErr := atomicWrite(m.configPath(), pending.Previous)
	liveErr := m.applyLiveConfig(ctx, state, pending.Previous, pending.liveConfig)
	if err := errors.Join(diskErr, liveErr); err != nil {
		return errors.Join(cause, i18n.Wrap("Could not restore the previous running configuration: %s", err))
	}
	if err := os.Remove(m.pendingPath()); err != nil {
		return errors.Join(cause, err)
	}
	var status controllerStatus
	if !errors.As(cause, &status) {
		return i18n.Wrap("Rule apply failed; previous saved configuration was restored, but the running state is unconfirmed. Restart Mihomo before retrying: %s", cause)
	}
	return i18n.Wrap("Reload failed; restored previous configuration: %s", cause)
}
