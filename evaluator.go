package toggletown

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

// hashString hashes a string to a number between 0-99 for rollout bucketing.
// Uses SHA-256 for consistency with Node.js and browser SDKs.
func hashString(input string) int {
	h := sha256.Sum256([]byte(input))
	num := uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
	return int(num % 100)
}

// isInRollout checks if a user is in the rollout percentage
func isInRollout(userID, flagKey string, percentage int) bool {
	if percentage == 0 {
		return false
	}
	if percentage >= 100 {
		return true
	}
	bucket := hashString(fmt.Sprintf("%s:%s", userID, flagKey))
	return bucket < percentage
}

// matchesRule checks if context matches a targeting rule
func matchesRule(rule Rule, context map[string]interface{}) bool {
	attribute := rule.Attribute
	operator := rule.Operator
	ruleValue := rule.Value

	// Get attribute value from context
	attrValue, ok := context[attribute]
	if !ok {
		// Check nested attributes
		if attrs, ok := context["attributes"].(map[string]interface{}); ok {
			attrValue, ok = attrs[attribute]
			if !ok {
				return false
			}
		} else {
			return false
		}
	}

	switch operator {
	case "equals":
		return fmt.Sprintf("%v", attrValue) == fmt.Sprintf("%v", ruleValue)
	case "not_equals":
		return fmt.Sprintf("%v", attrValue) != fmt.Sprintf("%v", ruleValue)
	case "contains":
		attrStr, ok1 := attrValue.(string)
		ruleStr, ok2 := ruleValue.(string)
		if ok1 && ok2 {
			return strings.Contains(attrStr, ruleStr)
		}
		return false
	case "not_contains":
		attrStr, ok1 := attrValue.(string)
		ruleStr, ok2 := ruleValue.(string)
		if ok1 && ok2 {
			return !strings.Contains(attrStr, ruleStr)
		}
		return false
	case "gt":
		attrFloat := toFloat(attrValue)
		ruleFloat := toFloat(ruleValue)
		return attrFloat > ruleFloat
	case "lt":
		attrFloat := toFloat(attrValue)
		ruleFloat := toFloat(ruleValue)
		return attrFloat < ruleFloat
	case "in":
		if list, ok := ruleValue.([]interface{}); ok {
			for _, item := range list {
				if fmt.Sprintf("%v", attrValue) == fmt.Sprintf("%v", item) {
					return true
				}
			}
		}
		return false
	case "not_in":
		if list, ok := ruleValue.([]interface{}); ok {
			for _, item := range list {
				if fmt.Sprintf("%v", attrValue) == fmt.Sprintf("%v", item) {
					return false
				}
			}
		}
		return true
	case "always":
		return true
	}

	return false
}

// toFloat converts various types to float64
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// getOffValue returns the "off" value for a flag type
func getOffValue(flagType string) interface{} {
	switch flagType {
	case "BOOLEAN":
		return false
	case "STRING":
		return ""
	case "NUMBER":
		return float64(0)
	case "JSON":
		return nil
	default:
		return nil
	}
}

// evaluateFlag evaluates a flag for a given context
func evaluateFlag(config FlagConfig, context map[string]interface{}) interface{} {
	if !config.Enabled {
		return config.DefaultValue
	}

	// Get user_id for rollout
	userID := ""
	if id, ok := context["user_id"].(string); ok {
		userID = id
	} else if id, ok := context["userId"].(string); ok {
		userID = id
	}

	// Check targeting rules in order
	for _, rule := range config.Rules {
		if !matchesRule(rule, context) {
			continue
		}

		// If rule has a percentage rollout, check if user qualifies
		if rule.Percentage != nil && *rule.Percentage < 100 {
			if userID == "" || !isInRollout(userID, config.Key, *rule.Percentage) {
				continue
			}
		}

		if rule.RollValue != nil {
			return rule.RollValue
		}
		return config.DefaultValue
	}

	// No rules matched - check global percentage rollout
	if config.RolloutPercentage > 0 && userID != "" {
		if isInRollout(userID, config.Key, config.RolloutPercentage) {
			return config.DefaultValue
		}
		return getOffValue(config.Type)
	}

	return config.DefaultValue
}
