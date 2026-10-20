package telemetry

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/protocol"
)

func TestWritePrometheusRendersExpectedFamilies(t *testing.T) {
	monitor := Monitor()
	monitor.Record("/v1/chat/completions", 200, 120*time.Millisecond, &RequestMeta{
		Model: "model-a", Tier: "go", ServerKeyID: "caller", Protocol: protocol.Chat,
		Usage: protocol.Usage{Input: 11, Output: 5, Total: 16}, UsageReported: true, CostUSD: 0.25,
	})
	monitor.RecordAttempt(UpstreamAttempt{
		Time: time.Now(), Model: "model-a", Tier: "go", Channel: "key",
		KeyID: "abcde", Status: 200, Success: true, Outcome: "success",
	})

	var buffer bytes.Buffer
	cluster := PrometheusCluster{
		Version: "v1.2.3", ModelsTotal: 10, ModelsExposed: 4, ModelsStale: true,
		ZenKeys: 2, GoKeys: 1, KeysCooling: 1, ProxiesHealthy: 1, ProxiesUnhealthy: 1,
	}
	if err := monitor.WritePrometheus(&buffer, cluster); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	output := buffer.String()
	for _, want := range []string{
		`opencode2api_build_info{version="v1.2.3"} 1`,
		"# TYPE opencode2api_requests_total counter",
		`opencode2api_requests_total{result="success"} 1`,
		`opencode2api_requests_by_endpoint_total{endpoint="/v1/chat/completions"} 1`,
		`opencode2api_requests_by_model_total{model="model-a"} 1`,
		`opencode2api_tokens_total{type="input"} 11`,
		`opencode2api_tokens_total{type="total"} 16`,
		`opencode2api_server_key_requests_total{server_key="caller"} 1`,
		`opencode2api_cost_usd_total 0.25`,
		`opencode2api_server_key_cost_usd_total{server_key="caller"} 0.25`,
		`opencode2api_upstream_attempts_total{result="success"} 1`,
		`opencode2api_upstream_attempts_by_key_total{key="abcde",result="success"} 1`,
		"opencode2api_models_stale 1",
		`opencode2api_upstream_keys{pool="zen"} 2`,
		`opencode2api_proxies{health="unhealthy"} 1`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("exposition is missing %q", want)
		}
	}
}

func TestWritePrometheusEscapesLabelValues(t *testing.T) {
	monitor := Monitor()
	monitor.Record(`/v1/"quoted"\path`, 200, time.Millisecond, &RequestMeta{})
	var buffer bytes.Buffer
	if err := monitor.WritePrometheus(&buffer, PrometheusCluster{}); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	if !strings.Contains(buffer.String(), `endpoint="/v1/\"quoted\"\\path"`) {
		t.Fatalf("label value was not escaped:\n%s", buffer.String())
	}
}

func TestFormatFloatKeepsIntegersPlain(t *testing.T) {
	cases := map[float64]string{
		0:          "0",
		1792506086: "1792506086",
		0.05:       "0.05",
		1.5:        "1.5",
	}
	for value, want := range cases {
		if got := formatFloat(value); got != want {
			t.Errorf("formatFloat(%v) = %q, want %q", value, got, want)
		}
	}
}
