package toggletown

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Shared with every SDK: tests/contract/flag-evaluation.json
type contractCase struct {
	Name     string                 `json:"name"`
	Flag     FlagConfig             `json:"flag"`
	Context  map[string]interface{} `json:"context"`
	Expected interface{}            `json:"expected"`
}

func TestFlagEvaluationContract(t *testing.T) {
	// Monorepo: <root>/tests/contract. Standalone sdk-go repo: ./tests/contract.
	var raw []byte
	var err error
	for _, dir := range []string{"tests/contract", "../../tests/contract"} {
		if raw, err = os.ReadFile(filepath.Join(dir, "flag-evaluation.json")); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []contractCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}

	for _, c := range fixture.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			// The Go SDK reads user_id and attributes from one flat map
			ctx := map[string]interface{}{}
			if attrs, ok := c.Context["attributes"].(map[string]interface{}); ok {
				for k, v := range attrs {
					ctx[k] = v
				}
			}
			if id, ok := c.Context["userId"]; ok {
				ctx["user_id"] = id
			}

			got := evaluateFlag(c.Flag, ctx)
			// JSON decodes every number as float64; compare the encoded form so 5 and 5.0 agree
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(c.Expected)
			assert.JSONEq(t, string(wantJSON), string(gotJSON))
		})
	}
}
