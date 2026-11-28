package styles

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

// The test executable acts as a protocol fixture without invoking fonts or UI.
// A valid receipt with excessive leading whitespace is still oversized output.
func TestMain(m *testing.M) {
	if mode := os.Getenv("ONESUB_STYLE_PROTOCOL_FIXTURE"); mode != "" {
		var request struct{ RequestID, OutputPath string }
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(request.OutputPath, []byte(`{}`), 0600); err != nil {
			os.Exit(3)
		}
		if mode == "oversized" {
			os.Stdout.Write(bytes.Repeat([]byte(" "), 1024*1024+1))
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": 1, "requestId": request.RequestID, "ok": true})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStyleWorkerBoundsControlOutput(t *testing.T) {
	for _, mode := range []string{"normal", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ONESUB_STYLE_PROTOCOL_FIXTURE", mode)
			client := Client{Executable: os.Args[0], Scratch: t.TempDir()}
			_, err := client.Catalog(context.Background())
			if (err != nil) != (mode == "oversized") {
				t.Fatal("unexpected control output result", mode, err)
			}
		})
	}
}
