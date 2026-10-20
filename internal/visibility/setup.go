// Package visibility описывает поля для поиска операций в Temporal UI.
package visibility

import (
	"context"
	"fmt"

	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/sdk/temporal"
)

var (
	OperationKind = temporal.NewSearchAttributeKeyKeyword("OperationKind")
	VMID          = temporal.NewSearchAttributeKeyKeyword("VMId")
	UserID        = temporal.NewSearchAttributeKeyKeyword("UserId")
)

// Setup выполняется до outbox: незарегистрированные поля мешают запуску workflow.
func Setup(ctx context.Context, operator operatorservice.OperatorServiceClient) error {
	attributes, err := operator.ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{Namespace: "default"})
	if err != nil {
		return fmt.Errorf("list search attributes: %w", err)
	}
	missing := make(map[string]enums.IndexedValueType)
	for _, name := range []string{OperationKind.GetName(), VMID.GetName(), UserID.GetName()} {
		attributeType, exists := attributes.GetCustomAttributes()[name]
		if !exists {
			missing[name] = enums.INDEXED_VALUE_TYPE_KEYWORD
			continue
		}
		if attributeType != enums.INDEXED_VALUE_TYPE_KEYWORD {
			return fmt.Errorf("search attribute %s: expected Keyword, got %s", name, attributeType)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	_, err = operator.AddSearchAttributes(ctx, &operatorservice.AddSearchAttributesRequest{
		Namespace:        "default",
		SearchAttributes: missing,
	})
	if err != nil {
		return fmt.Errorf("add search attributes: %w", err)
	}
	return nil
}
