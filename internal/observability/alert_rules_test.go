package observability

import (
	_ "embed"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The alert rules and their promtool unit tests (`just test-alerts`); the thresholds are proven
// there, this test guards the structure and the metric names the rules depend on.
var (
	//go:embed alerts/rmi-alerts.rules.yml
	alertRulesFile []byte
	//go:embed alerts/rmi-alerts.test.yml
	alertRulesTestFile []byte
)

type alertRuleFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Record      string            `yaml:"record"`
			Expr        string            `yaml:"expr"`
			For         string            `yaml:"for"`
			Labels      map[string]string `yaml:"labels"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

type alertRuleTestFile struct {
	RuleFiles []string `yaml:"rule_files"`
	Tests     []struct {
		AlertRuleTest []struct {
			Alertname string `yaml:"alertname"`
			ExpAlerts []any  `yaml:"exp_alerts"`
		} `yaml:"alert_rule_test"`
	} `yaml:"tests"`
}

// rmiMetricRef matches the series of this service referenced by an expression.
var rmiMetricRef = regexp.MustCompile(`\b(?:app_rmi|rmi)_[a-z0-9_]+\b`)

var descFQName = regexp.MustCompile(`fqName: "([^"]+)"`)

// registeredRMIMetricNames returns the fully qualified names of the collectors in metrics.go.
func registeredRMIMetricNames(t *testing.T) map[string]bool {
	t.Helper()
	collectors := []prometheus.Collector{
		RequestDuration, CacheHits, DatabaseOperations, SelfDeclaredUpdates,
		PhoneVerificationRequests, PhoneVerificationValidations, ActiveConnections,
		OperationDuration, RMISyncQueueDepth, RMISyncOperationsTotal, RMISyncFailuresTotal,
		RMICacheHitRatio, RMIDegradedModeActive,
	}
	names := map[string]bool{}
	for _, c := range collectors {
		ch := make(chan *prometheus.Desc, 4)
		c.Describe(ch)
		close(ch)
		for d := range ch {
			m := descFQName.FindStringSubmatch(d.String())
			require.Len(t, m, 2, "desc %s", d.String())
			names[m[1]] = true
		}
	}
	return names
}

func TestAlertRules(t *testing.T) {
	var rules alertRuleFile
	require.NoError(t, yaml.Unmarshal(alertRulesFile, &rules))
	require.NotEmpty(t, rules.Groups)

	metrics := registeredRMIMetricNames(t)
	seen := map[string]bool{}
	for _, g := range rules.Groups {
		require.NotEmpty(t, g.Name)
		for _, r := range g.Rules {
			require.NotEmpty(t, r.Alert, "group %s: only alerting rules belong here", g.Name)
			assert.Empty(t, r.Record, r.Alert)
			assert.False(t, seen[r.Alert], "duplicated alert %s", r.Alert)
			seen[r.Alert] = true

			assert.NotEmpty(t, strings.TrimSpace(r.Expr), r.Alert)
			assert.NotEmpty(t, r.For, "%s: set `for` explicitly", r.Alert)
			assert.Contains(t, []string{"warning", "critical"}, r.Labels["severity"], r.Alert)
			assert.Equal(t, "rmi", r.Labels["service"], r.Alert)
			for _, a := range []string{"summary", "description", "runbook_url"} {
				assert.NotEmpty(t, r.Annotations[a], "%s: annotation %s", r.Alert, a)
			}

			for _, ref := range rmiMetricRef.FindAllString(r.Expr, -1) {
				base := ref
				for _, suffix := range []string{"_bucket", "_count", "_sum"} {
					base = strings.TrimSuffix(base, suffix)
				}
				assert.True(t, metrics[base], "%s references %s, which metrics.go does not export", r.Alert, ref)
			}
		}
	}

	for _, want := range []string{
		"RMIHighErrorRate", "RMIHighLatencyP95", "RMIPodRestarting", "RMIPodCrashLooping",
		"RMISyncQueueBacklog", "RMISyncDLQNewItems",
	} {
		assert.True(t, seen[want], "missing alert %s", want)
	}

	// Every alert has a promtool case in which it fires.
	var tests alertRuleTestFile
	require.NoError(t, yaml.Unmarshal(alertRulesTestFile, &tests))
	assert.Equal(t, []string{"rmi-alerts.rules.yml"}, tests.RuleFiles)
	fires := map[string]bool{}
	for _, tc := range tests.Tests {
		for _, at := range tc.AlertRuleTest {
			assert.True(t, seen[at.Alertname], "test of unknown alert %s", at.Alertname)
			if len(at.ExpAlerts) > 0 {
				fires[at.Alertname] = true
			}
		}
	}
	for alert := range seen {
		assert.True(t, fires[alert], "alert %s has no promtool case in which it fires", alert)
	}
}
