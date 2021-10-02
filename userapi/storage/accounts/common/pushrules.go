package common

import (
	"encoding/json"
	"strings"
	"text/template"
)

func DefaultPushRules(userID, serverName string) (json.RawMessage, error) {
	var sb strings.Builder
	if err := pushRulesTmpl.Execute(&sb, map[string]string{"UserID": userID, "ServerName": serverName}); err != nil {
		return nil, err
	}
	return json.RawMessage(sb.String()), nil
}

// pushRulesTmpl is a simple value of m.push_rules. It was
// snapshotted from a default Element Web account on matrix.org.
var pushRulesTmpl = template.Must(template.New("").Parse(`{
	"device" : {},
	"global" : {
	   "content" : [
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "sound",
	               "value" : "default"
	            },
	            {
	               "set_tweak" : "highlight"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "pattern" : "{{.Device.UserID}}",
	         "rule_id" : ".m.rule.contains_user_name"
	      }
	   ],
	   "override" : [
	      {
	         "actions" : [
	            "dont_notify"
	         ],
	         "conditions" : [],
	         "default" : true,
	         "enabled" : false,
	         "rule_id" : ".m.rule.master"
	      },
	      {
	         "actions" : [
	            "dont_notify"
	         ],
	         "conditions" : [
	            {
	               "key" : "content.msgtype",
	               "kind" : "event_match",
	               "pattern" : "m.notice"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.suppress_notices"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "sound",
	               "value" : "default"
	            },
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.member"
	            },
	            {
	               "key" : "content.membership",
	               "kind" : "event_match",
	               "pattern" : "invite"
	            },
	            {
	               "key" : "state_key",
	               "kind" : "event_match",
	               "pattern" : "@{{.Device.UserID}}:{{.ServerName}}"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.invite_for_me"
	      },
	      {
	         "actions" : [
	            "dont_notify"
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.member"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.member_event"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "sound",
	               "value" : "default"
	            },
	            {
	               "set_tweak" : "highlight"
	            }
	         ],
	         "conditions" : [
	            {
	               "kind" : "contains_display_name"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.contains_display_name"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "highlight",
	               "value" : true
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "content.body",
	               "kind" : "event_match",
	               "pattern" : "@room"
	            },
	            {
	               "key" : "room",
	               "kind" : "sender_notification_permission"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.roomnotif"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "highlight",
	               "value" : true
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.tombstone"
	            },
	            {
	               "key" : "state_key",
	               "kind" : "event_match",
	               "pattern" : ""
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.tombstone"
	      },
	      {
	         "actions" : [
	            "dont_notify"
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.reaction"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.reaction"
	      }
	   ],
	   "room" : [],
	   "sender" : [],
	   "underride" : [
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "sound",
	               "value" : "ring"
	            },
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.call.invite"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.call"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "sound",
	               "value" : "default"
	            },
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "is" : "2",
	               "kind" : "room_member_count"
	            },
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.message"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.room_one_to_one"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "sound",
	               "value" : "default"
	            },
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "is" : "2",
	               "kind" : "room_member_count"
	            },
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.encrypted"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.encrypted_room_one_to_one"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.message"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.message"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "m.room.encrypted"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".m.rule.encrypted"
	      },
	      {
	         "actions" : [
	            "notify",
	            {
	               "set_tweak" : "highlight",
	               "value" : false
	            }
	         ],
	         "conditions" : [
	            {
	               "key" : "type",
	               "kind" : "event_match",
	               "pattern" : "im.vector.modular.widgets"
	            },
	            {
	               "key" : "content.type",
	               "kind" : "event_match",
	               "pattern" : "jitsi"
	            },
	            {
	               "key" : "state_key",
	               "kind" : "event_match",
	               "pattern" : "*"
	            }
	         ],
	         "default" : true,
	         "enabled" : true,
	         "rule_id" : ".im.vector.jitsi"
	      }
	   ]
	}
}`))
