package security

import (
	"strings"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/util"
)

type ValueDecoder func(map[string]any, string) (string, problems.Problem)

// ValuesDecoders is a map of custom decoders for values in the claims map
var defaultValuesDecoders = map[string]ValueDecoder{
	"group": func(m map[string]any, s string) (string, problems.Problem) {
		client, ok := util.NotNilOfType[string](m["clientId"])
		if !ok {
			return "", invalidCtx
		}
		parts := strings.Split(client, "--")
		if len(parts) != 2 {
			return "", invalidCtx
		}
		if parts[1] == "" {
			return "", invalidCtx
		}
		return parts[0], nil
	},
	"team": func(m map[string]any, s string) (string, problems.Problem) {
		client, ok := util.NotNilOfType[string](m["clientId"])
		if !ok {
			return "", invalidCtx
		}
		parts := strings.Split(client, "--")
		if len(parts) != 2 {
			return "", invalidCtx
		}
		if parts[1] == "" {
			return "", invalidCtx
		}
		return parts[1], nil
	},
}

// DecodeValue decodes a value from the claims map using a custom decoder if available
// If not, it will try to get the value directly from the claims map
func DecodeValue(decoders map[string]ValueDecoder, claims map[string]any, key string) (string, problems.Problem) {
	decoder, ok := decoders[key]
	if !ok {
		val, ok := util.GetValue[string](claims, key)
		if !ok || val == "" {
			return "", invalidCtxField(key)
		}
		return val, nil
	}
	return decoder(claims, key)
}
