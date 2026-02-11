package toggletown

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashString(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		assert.Equal(t, hashString("test"), hashString("test"))
	})

	t.Run("range 0-99", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			result := hashString(fmt.Sprintf("user-%d", i))
			assert.GreaterOrEqual(t, result, 0)
			assert.Less(t, result, 100)
		}
	})
}

func TestIsInRollout(t *testing.T) {
	t.Run("zero percent", func(t *testing.T) {
		assert.False(t, isInRollout("user-123", "flag", 0))
	})

	t.Run("hundred percent", func(t *testing.T) {
		assert.True(t, isInRollout("user-123", "flag", 100))
	})

	t.Run("deterministic", func(t *testing.T) {
		result1 := isInRollout("user-123", "my-flag", 50)
		result2 := isInRollout("user-123", "my-flag", 50)
		assert.Equal(t, result1, result2)
	})
}

func TestMatchesRule(t *testing.T) {
	t.Run("equals", func(t *testing.T) {
		rule := Rule{Attribute: "plan", Operator: "equals", Value: "pro"}
		assert.True(t, matchesRule(rule, map[string]interface{}{"plan": "pro"}))
		assert.False(t, matchesRule(rule, map[string]interface{}{"plan": "free"}))
	})

	t.Run("not_equals", func(t *testing.T) {
		rule := Rule{Attribute: "plan", Operator: "not_equals", Value: "free"}
		assert.True(t, matchesRule(rule, map[string]interface{}{"plan": "pro"}))
		assert.False(t, matchesRule(rule, map[string]interface{}{"plan": "free"}))
	})

	t.Run("contains", func(t *testing.T) {
		rule := Rule{Attribute: "email", Operator: "contains", Value: "@company.com"}
		assert.True(t, matchesRule(rule, map[string]interface{}{"email": "user@company.com"}))
		assert.False(t, matchesRule(rule, map[string]interface{}{"email": "user@other.com"}))
	})

	t.Run("gt", func(t *testing.T) {
		rule := Rule{Attribute: "age", Operator: "gt", Value: float64(18)}
		assert.True(t, matchesRule(rule, map[string]interface{}{"age": float64(25)}))
		assert.False(t, matchesRule(rule, map[string]interface{}{"age": float64(15)}))
	})

	t.Run("in list", func(t *testing.T) {
		rule := Rule{Attribute: "country", Operator: "in", Value: []interface{}{"US", "CA", "UK"}}
		assert.True(t, matchesRule(rule, map[string]interface{}{"country": "US"}))
		assert.False(t, matchesRule(rule, map[string]interface{}{"country": "DE"}))
	})
}

func TestEvaluateFlag(t *testing.T) {
	t.Run("disabled flag returns off value", func(t *testing.T) {
		config := FlagConfig{Type: "BOOLEAN", Enabled: false, DefaultValue: true}
		assert.Equal(t, false, evaluateFlag(config, map[string]interface{}{}))
	})

	t.Run("enabled boolean with 100% rollout", func(t *testing.T) {
		config := FlagConfig{
			Type:              "BOOLEAN",
			Enabled:           true,
			DefaultValue:      true,
			RolloutPercentage: 100,
			Rules:             []Rule{},
		}
		assert.Equal(t, true, evaluateFlag(config, map[string]interface{}{"user_id": "123"}))
	})

	t.Run("targeting rule match", func(t *testing.T) {
		config := FlagConfig{
			Type:              "BOOLEAN",
			Enabled:           true,
			DefaultValue:      false,
			RolloutPercentage: 0,
			Rules: []Rule{
				{Attribute: "plan", Operator: "equals", Value: "pro"},
			},
		}
		assert.Equal(t, true, evaluateFlag(config, map[string]interface{}{"plan": "pro"}))
		assert.Equal(t, false, evaluateFlag(config, map[string]interface{}{"plan": "free"}))
	})

	t.Run("string flag", func(t *testing.T) {
		config := FlagConfig{
			Type:              "STRING",
			Enabled:           true,
			DefaultValue:      "variant-a",
			RolloutPercentage: 100,
			Rules:             []Rule{},
		}
		assert.Equal(t, "variant-a", evaluateFlag(config, map[string]interface{}{"user_id": "123"}))
	})

	t.Run("number flag", func(t *testing.T) {
		config := FlagConfig{
			Type:              "NUMBER",
			Enabled:           true,
			DefaultValue:      float64(42),
			RolloutPercentage: 100,
			Rules:             []Rule{},
		}
		assert.Equal(t, float64(42), evaluateFlag(config, map[string]interface{}{"user_id": "123"}))
	})
}
