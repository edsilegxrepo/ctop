package jsonfmt

import (
	"strings"
	"testing"
)

func TestFormatLogMessage(t *testing.T) {
	// Plain text log line
	plain := "Standard application startup message"
	if out := FormatLogMessage(plain); out != plain {
		t.Fatalf("expected '%s', got '%s'", plain, out)
	}

	// JSON log line
	jsonInput := `{"level":"info","msg":"HTTP request","req":{"method":"GET","status":200,"path":"/health"}}`
	formatted := FormatLogMessage(jsonInput)

	if !strings.Contains(formatted, "level=info") {
		t.Errorf("expected formatted log to contain 'level=info', got: %s", formatted)
	}
	if !strings.Contains(formatted, "req.method=GET") {
		t.Errorf("expected formatted log to contain 'req.method=GET', got: %s", formatted)
	}
	if !strings.Contains(formatted, "req.status=200") {
		t.Errorf("expected formatted log to contain 'req.status=200', got: %s", formatted)
	}
	if !strings.Contains(formatted, "req.path=/health") {
		t.Errorf("expected formatted log to contain 'req.path=/health', got: %s", formatted)
	}
}
