package visibility

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
)

type testOperator struct {
	operatorservice.OperatorServiceClient
	attributes map[string]enums.IndexedValueType
	added      map[string]enums.IndexedValueType
	listErr    error
	addErr     error
}

func (operator *testOperator) ListSearchAttributes(context.Context, *operatorservice.ListSearchAttributesRequest, ...grpc.CallOption) (*operatorservice.ListSearchAttributesResponse, error) {
	return &operatorservice.ListSearchAttributesResponse{CustomAttributes: operator.attributes}, operator.listErr
}

func (operator *testOperator) AddSearchAttributes(_ context.Context, request *operatorservice.AddSearchAttributesRequest, _ ...grpc.CallOption) (*operatorservice.AddSearchAttributesResponse, error) {
	operator.added = request.SearchAttributes
	operator.attributes = request.SearchAttributes
	return &operatorservice.AddSearchAttributesResponse{}, operator.addErr
}

func TestSetup(t *testing.T) {
	operator := &testOperator{}
	require.NoError(t, Setup(context.Background(), operator))
	require.Equal(t, map[string]enums.IndexedValueType{
		"OperationKind": enums.INDEXED_VALUE_TYPE_KEYWORD,
		"VMId":          enums.INDEXED_VALUE_TYPE_KEYWORD,
		"UserId":        enums.INDEXED_VALUE_TYPE_KEYWORD,
	}, operator.added)
	operator.added = nil
	require.NoError(t, Setup(context.Background(), operator))
	require.Nil(t, operator.added)
	operator.attributes["UserId"] = enums.INDEXED_VALUE_TYPE_TEXT
	require.ErrorContains(t, Setup(context.Background(), operator), "UserId")
	require.Nil(t, operator.added)
}

func TestSetupFailures(t *testing.T) {
	for _, operator := range []*testOperator{
		{listErr: errors.New("offline")},
		{addErr: errors.New("registration failed")},
	} {
		require.Error(t, Setup(context.Background(), operator))
	}
}
