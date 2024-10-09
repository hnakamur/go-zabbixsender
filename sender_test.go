package zabbix

import (
	"strings"
	"testing"
)

func TestParseResponse(t *testing.T) {
	respPacket := "ZBXD\x01" + "\x5A\x00\x00\x00" + "\x00\x00\x00\x00" +
		`{"response":"success","info":"processed: 1; failed: 0; total: 1; seconds spent: 0.060753"}`
	resp, err := parseResponse(strings.NewReader(respPacket))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resp.IsSucccess(), true; got != want {
		t.Errorf("resp.IsSuccess mismatch, got=%v, want=%v", got, want)
	}
	if got, want := resp.Processed, 1; got != want {
		t.Errorf("resp.Processed mismatch, got=%v, want=%v", got, want)
	}
	if got, want := resp.Total, 1; got != want {
		t.Errorf("resp.Total mismatch, got=%v, want=%v", got, want)
	}
	if got, want := resp.SecondsSpent, 0.060753; got != want {
		t.Errorf("resp.SecondsSpent mismatch, got=%v, want=%v", got, want)
	}
}
