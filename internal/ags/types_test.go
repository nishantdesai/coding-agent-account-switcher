package ags

import (
	"testing"
	"time"
)

func TestParseToolAndString(t *testing.T) {
	for _, tool := range []Tool{ToolCodex, ToolClaude, ToolPi} {
		parsed, ok := ParseTool(tool.String())
		if !ok || parsed != tool {
			t.Fatalf("expected parse success for %q", tool)
		}
	}

	if _, ok := ParseTool("unknown"); ok {
		t.Fatalf("expected parse failure")
	}
}

func TestDefaultStateAndNowISO(t *testing.T) {
	st := defaultState()
	if st.Version != 1 || len(st.Entries) != 0 {
		t.Fatalf("unexpected default state: %+v", st)
	}

	n := nowUTC()
	if n.Location() != time.UTC {
		t.Fatalf("expected UTC time")
	}

	iso := nowISO()
	if _, err := time.Parse(time.RFC3339, iso); err != nil {
		t.Fatalf("expected RFC3339 timestamp, got %q (%v)", iso, err)
	}
}
