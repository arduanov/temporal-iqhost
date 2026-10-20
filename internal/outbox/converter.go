package outbox

import (
	common "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/converter"
)

func clientData(p *common.Payload, v interface{}) error {
	return converter.GetDefaultDataConverter().FromPayload(p, v)
}
