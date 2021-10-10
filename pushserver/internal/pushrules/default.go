package pushrules

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// DefaultRuleSet returns the default ruleset for a given (fully
// qualified) MXID.
func DefaultRuleSet(localpart string, serverName gomatrixserverlib.ServerName) *RuleSet {
	return &RuleSet{
		Override:  defaultOverrideRules("@" + localpart + ":" + string(serverName)),
		Content:   defaultContentRules(localpart),
		Underride: defaultUnderrideRules,
	}
}
